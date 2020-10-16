package bridgenode

import (
	"encoding/binary"
	"fmt"
	"os"
	"sync"

	"github.com/mit-dci/utreexo/util"
)

/*
Proof file format is somewhat like the blk.dat and rev.dat files.  But it's
always in order!  The offset file is in 8 byte chunks, so to find the proof
data for block 100 (really 101), seek to byte 800 and read 8 bytes.

The proof file is: 4 bytes empty (zeros for now, could do something else later)
4 bytes proof length, then the proof data.

Offset file is: 8 byte int64 offset.  Right now it's all 1 big file, can
change to 4 byte which file and 4 byte offset within file like the blk/rev but
we're not running on fat32 so works OK for now.

the offset file will start with 16 zero-bytes.  The first offset is 0 because
there is no block 0.  The next is 0 because block 1 starts at byte 0 of proof.dat.
then the second offset, at byte 16, is 12 or so, as that's block 2 in the proof.dat.
*/

/*
There are 2 worker threads writing to the flat file.
(None of them read from it).

	flatFileBlockWorker gets proof blocks from the proofChan, writes everthing
to disk (including the offset file) and also sends the offset over a channel
to the ttl worker.
When flatFileBlockWorker first starts it tries to read the entire offset file
and send it over to the ttl worker.

	flatFileTTLWorker gets blocks of TTL values from the ttlResultChan.  It
also gets offset values from flatFileBlockWorker so it knows it's safe to write
to those locations.
Then it writes all the TTL values to the correct places in by checking all the
offsetInRam values and writing to the correct 4-byte location in the proof file.

*/

// shared state for the flat file worker methods
type flatFileState struct {
	offsets               []int64
	proofFile, offsetFile *os.File
	currentHeight         int32
	currentOffset         int64
	fileWait              *sync.WaitGroup
}

// pFileWorker takes in blockproof and height information from the channel
// and writes to disk. MUST NOT have more than one worker as the proofs need to be
// in order
func flatFileWorker(
	proofChan chan util.UData,
	ttlResultChan chan ttlResultBlock,
	fileWait *sync.WaitGroup) {

	var ff flatFileState
	var err error

	ff.offsetFile, err = os.OpenFile(
		util.POffsetFilePath, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		panic(err)
	}

	ff.proofFile, err = os.OpenFile(
		util.PFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}

	ff.fileWait = fileWait

	ff.ffInit()

	// Grab either proof bytes and write em to offset / proof file, OR, get a TTL result
	// and write that.  Will this lock up if it keeps doing proofs and ignores ttls?
	// it should keep both buffers about even.  If it keeps doing proofs and the ttl
	// buffer fills, then eventually it'll block...?
	// Also, is it OK to have 2 different workers here?  It probably is, with the
	// ttl side having read access to the proof writing side's last written proof.
	// then the TTL side can do concurrent writes.  Also the TTL writes might be
	// slow since they're all over the place.  Also the offsets should definitely
	// be in ram since they'll be accessed a lot.

	// TODO ^^^^^^ all that stuff.

	// main selector - Write block proofs whenever you get them
	// if you get TTLs, write them only if they're not too high
	// if they are too high, keep writing proof blocks until they're not
	for {
		select {
		case ud := <-proofChan:
			ff.writeProofBlock(ud)
		case ttlRes := <-ttlResultChan:
			for ttlRes.Height > ff.currentHeight {
				ud := <-proofChan
				ff.writeProofBlock(ud)
			}
			ff.writeTTLs(ttlRes)
		}
	}
}

func (ff *flatFileState) ffInit() error {
	// seek to end to get the number of offsets in the file (# of blocks)
	offsetFileSize, err := ff.offsetFile.Seek(0, 2)
	if err != nil {
		return err
	}
	if offsetFileSize%8 != 0 {
		return fmt.Errorf("offset file not mulitple of 8 bytes")
	}

	// resume setup -- read all existing offsets to ram
	if offsetFileSize > 0 {
		// offsetFile already exists so read the whole thing and send over the
		// channel to the ttl worker.
		maxHeight := int32(offsetFileSize / 8)
		// seek back to the file start / block "0"
		_, err := ff.offsetFile.Seek(0, 0)
		if err != nil {
			return err
		}

		// run through the file, read everything and push into the channel
		for ff.currentHeight < maxHeight {
			err = binary.Read(ff.offsetFile, binary.BigEndian, &ff.currentOffset)
			if err != nil {
				fmt.Printf("couldn't populate in-ram offsets on startup")
				return err
			}
			ff.offsets[ff.currentHeight] = ff.currentOffset
		}
	} else { // first time startup
		// there is no block 0 so leave that empty
		fmt.Printf("setting h=1\n")
		_, err = ff.offsetFile.Write(make([]byte, 8))
		if err != nil {
			return err
		}
		// start writing at block 1
		ff.currentHeight = 1
	}
	return nil
}

func (ff *flatFileState) writeProofBlock(ud util.UData) {
	// note that we know the offset for block 2 once we're done writing block 1,
	// but we don't write the block 2 offset until we get block 2

	// get the new block proof
	// put offset in ram
	ff.offsets[ff.currentHeight] = ff.currentOffset
	// write to offset file so we can resume; offset file is only
	// read on startup and always incremented so we shouldn't need to seek
	err := binary.Write(ff.offsetFile, binary.BigEndian, ff.currentOffset)
	if err != nil {
		fmt.Printf(err.Error())
		return
	}

	// seek to next block proof location, this file is open in ttl worker
	// and may write point may have moved
	_, _ = ff.proofFile.Seek(ff.currentOffset, 0)

	info, _ := ff.proofFile.Stat()

	fmt.Printf("h %d wrote %d to offset file %d bytes long\n",
		ud.Height, ff.currentOffset, info.Size())

	pb := ud.ToBytes()

	// write to proof file

	// first write magic 4 bytes
	_, err = ff.proofFile.Write([]byte{0xaa, 0xff, 0xaa, 0xff})
	if err != nil {
		fmt.Printf(err.Error())
		return
	}

	// then write big endian proof size uint32 (proof never more than 4GB)
	err = binary.Write(ff.proofFile, binary.BigEndian, uint32(len(pb)))
	if err != nil {
		fmt.Printf(err.Error())
		return
	}

	// then write the proof
	written, err := ff.proofFile.Write(pb)
	if err != nil {
		fmt.Printf(err.Error())
		return
	}
	// arbitrary 32 byte gap between proof blocks
	ff.currentOffset += int64(written) + 8 // 4B magic & 4B size comes first
	ff.currentHeight++

	fmt.Printf(" len(pb)=%d wrote %d\n", len(pb), written)

	ff.currentHeight++

	// fmt.Printf("flatFileBlockWorker h %d done\n", proofWriteHeight)
}

func (ff *flatFileState) writeTTLs(ttlRes ttlResultBlock) error {
	var ttlArr [4]byte

	// for all the TTLs, seek and overwrite the empty values there
	for _, c := range ttlRes.Created {
		// seek to the location of that txo's ttl value in the proof file

		// fmt.Printf("flatFileTTLWorker write %d at offset %d\n",
		// 	ttlRes.Height-c.createHeight,
		// 	ff.offsets[c.createHeight]+4+int64(c.indexWithinBlock*4))

		binary.BigEndian.PutUint32(
			ttlArr[:], uint32(ttlRes.Height-c.createHeight))

		// write it's lifespan as a 4 byte int32 (bit of a waste as
		// 2 or 3 bytes would work)
		n, err := ff.proofFile.WriteAt(ttlArr[:],
			ff.offsets[c.createHeight]+4+int64(c.indexWithinBlock*4))
		if err != nil {
			return err
		}
		if n != 4 {
			return fmt.Errorf("non4 bytes")
		}
		// fmt.Printf("wrote ttl %d to blkh %d txo %d (byte %d)\n",
		// 	ttlRes.Height-c.createHeight, c.createHeight, c.indexWithinBlock,
		// 	inRamOffsets[c.createHeight]+4+
		// 		int64(c.indexWithinBlock*4))
	}
	ff.fileWait.Done()
	return nil
	// fmt.Printf("flatFileTTLWorker h %d done\n", ttlRes.Height)
}
