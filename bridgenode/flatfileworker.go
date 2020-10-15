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

// pFileWorker takes in blockproof and height information from the channel
// and writes to disk. MUST NOT have more than one worker as the proofs need to be
// in order
func flatFileBlockWorker(
	proofChan chan util.UData,
	offsetChan chan int64,
	fileWait *sync.WaitGroup) {

	// for the pFile
	proofFile, err := os.OpenFile(
		util.PFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}

	offsetFile, err := os.OpenFile(
		util.POffsetFilePath, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		panic(err)
	}

	// seek to end to get the number of offsets in the file (# of blocks)
	offsetMax, err := offsetFile.Seek(0, 2)
	if err != nil {
		panic(err)
	}
	if offsetMax%8 != 0 {
		panic("offset file not mulitple of 8 bytes")
	}

	var curOffset int64 // the last written location in the proof file

	// resume setup -- send lots of offsets to ttl worker
	if offsetMax > 0 {
		// offsetFile already exists so read the whole thing and send over the
		// channel to the ttl worker.

		// seek back to the file start / block 1
		offsetPos, err := offsetFile.Seek(8, 0)
		if err != nil {
			panic(err)
		}

		// run through the file, read everything and push into the channel
		for offsetPos < offsetMax {
			err = binary.Read(offsetFile, binary.BigEndian, &curOffset)
			if err != nil {
				fmt.Printf("couldn't populate in-ram offsets on startup")
				panic(err)
			}
			offsetChan <- curOffset
			offsetPos += 8
		}
	}

	proofWriteHeight := int32(curOffset / 8)

	if proofWriteHeight == 0 { // there is no block 0 so leave that empty
		fmt.Printf("setting h=1\n")
		proofWriteHeight = 1
		_, err = offsetFile.Write(make([]byte, 4))
		if err != nil {
			fmt.Printf(err.Error())
			return
		}
	}

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
	for {
		ud := <-proofChan
		// write to offset file first

		// note that seek does nothing!  Only seek up in the setup phase.

		err = binary.Write(offsetFile, binary.BigEndian, curOffset)
		if err != nil {
			fmt.Printf(err.Error())
			return
		}

		pb := ud.ToBytes()

		// write to proof file

		// first write magic 4 bytes
		_, err = proofFile.Write([]byte{0xaa, 0xff, 0xaa, 0xff})
		if err != nil {
			fmt.Printf(err.Error())
			return
		}

		// then write big endian proof size uint32 (proof never more than 4GB)
		err = binary.Write(proofFile, binary.BigEndian, uint32(len(pb)))
		if err != nil {
			fmt.Printf(err.Error())
			return
		}

		// then write the proof
		written, err := proofFile.Write(pb)
		if err != nil {
			fmt.Printf(err.Error())
			return
		}
		// arbitrary 32 byte gap between proof blocks
		curOffset += int64(written) + 8 // 4B magic & 4B size comes first
		proofWriteHeight++

		// send offset to the ttl worker after proofs are written to disk
		offsetChan <- curOffset
		fileWait.Done()
		// fmt.Printf("flatFileBlockWorker h %d done\n", proofWriteHeight)
	}
}

func flatFileTTLWorker(
	ttlResultChan chan ttlResultBlock,
	offsetChan chan int64,
	fileWait *sync.WaitGroup) {

	var inRamOffsets []int64

	// the offset for block 0 is 0?  kindof weird
	inRamOffsets = append(inRamOffsets, 0)

	maxOffsetHeight := int32(1) // start at 1?

	// for the pFile
	proofFile, err := os.OpenFile(
		util.PFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}

	for {
		// get offset data; if there is none, try getting ttl data
		select {
		case nextOffset := <-offsetChan:
			// got an offset, expand in ram offsets and write
			inRamOffsets = append(inRamOffsets, nextOffset)
			maxOffsetHeight++

		case ttlRes := <-ttlResultChan:
			// fmt.Printf("got ttlres h %d\n", ttlRes.Height)
			for ttlRes.Height > maxOffsetHeight {
				// we got a ttl result before the offset.  We need the offset data
				// first, so keep reading offsets until we're caught up
				inRamOffsets = append(inRamOffsets, <-offsetChan)
				maxOffsetHeight++
			}
			// for all the TTLs, seek and overwrite the empty values there
			for _, c := range ttlRes.Created {
				// seek to the location of that txo's ttl value in the proof file

				// fmt.Printf("want ram offset %d, only have up to %d\n",
				// 	c.createHeight, len(inRamOffsets))

				_, _ = proofFile.Seek(
					inRamOffsets[c.createHeight]+4+
						int64(c.indexWithinBlock*4), 0)
				// write it's lifespan as a 4 byte int32 (bit of a waste as
				// 2 or 3 bytes would work)
				_ = binary.Write(
					proofFile, binary.BigEndian, ttlRes.Height-c.createHeight)

				// fmt.Printf("wrote ttl %d to blkh %d txo %d (byte %d)\n",
				// 	ttlRes.Height-c.createHeight, c.createHeight, c.indexWithinBlock,
				// 	inRamOffsets[c.createHeight]+4+
				// 		int64(c.indexWithinBlock*4))
			}
			fileWait.Done()
			// fmt.Printf("flatFileTTLWorker h %d done\n", ttlRes.Height)
		}
	}
}
