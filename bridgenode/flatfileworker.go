package bridgenode

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"sync"

	"github.com/mit-dci/utreexo/accumulator"
	"github.com/mit-dci/utreexo/btcacc"
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

func flatFileWorkerProofBlocks(
	proofChan chan btcacc.UData,
	utreeDir utreeDir,
	fileWait *sync.WaitGroup) {

	var ff flatFileState
	var err error

	ff.offsetFile, err = os.OpenFile(
		utreeDir.ProofDir.pOffsetFile, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		panic(err)
	}

	ff.proofFile, err = os.OpenFile(
		utreeDir.ProofDir.pFile, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}

	ff.fileWait = fileWait

	err = ff.ffInit()
	if err != nil {
		panic(err)
	}

	for {
		ud := <-proofChan
		err = ff.writeProofBlock(ud)
		if err != nil {
			panic(err)
		}
	}
}

func flatFileWorkerUndoBlocks(
	undoChan chan accumulator.UndoBlock,
	utreeDir utreeDir,
	fileWait *sync.WaitGroup) {

	var uf flatFileState
	var err error

	uf.offsetFile, err = os.OpenFile(
		utreeDir.UndoDir.offsetFile, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		panic(err)
	}

	uf.proofFile, err = os.OpenFile(
		utreeDir.UndoDir.undoFile, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}

	uf.fileWait = fileWait

	err = uf.ffInit()
	if err != nil {
		panic(err)
	}
	for {
		undo := <-undoChan
		err = uf.writeUndoBlock(undo)
		if err != nil {
			panic(err)
		}

	}

}

func flatFileWorkerTTlBlocks(
	ttlResultChan chan ttlResultBlock,
	leafblockChan chan int,
	utreeDir utreeDir,
	fileWait *sync.WaitGroup) {

	var tf flatFileState
	var err error

	tf.offsetFile, err = os.OpenFile(
		utreeDir.TtlDir.OffsetFile, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		panic(err)
	}

	tf.proofFile, err = os.OpenFile(
		utreeDir.TtlDir.ttlsetFile, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		panic(err)
	}
	tf.fileWait = fileWait

	err = tf.ffInit()
	if err != nil {
		panic(err)
	}

	for {
		size := <-leafblockChan
		// 1 extra byte for numTTLs in the beginning
		bytesTtlWrite := make([]byte, (size+1)*4)
		binary.BigEndian.PutUint32(bytesTtlWrite[:4], uint32(size))
		_, err = tf.proofFile.WriteAt(bytesTtlWrite, tf.currentOffset)
		tf.fileWait.Done()
		if err != nil {
			panic(err)
		}
		ttlRes := <-ttlResultChan
		err = tf.writeTTLs(ttlRes)
		if err != nil {
			panic(err)
		}
		// append tf offsets after writing ttl data
		tf.offsets = append(tf.offsets, tf.currentOffset)
		// increment currentoffset value
		tf.currentOffset += int64(size+1) * 4
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
		ff.offsets = make([]int64, maxHeight)
		// run through the file, read everything and push into the channel
		for ff.currentHeight < maxHeight {
			err = binary.Read(ff.offsetFile, binary.BigEndian, &ff.currentOffset)
			if err != nil {
				fmt.Printf("couldn't populate in-ram offsets on startup")
				return err
			}
			ff.offsets[ff.currentHeight] = ff.currentOffset
			ff.currentHeight++
		}

		// set currentOffset to the end of the proof file
		ff.currentOffset, _ = ff.proofFile.Seek(0, 2)

	} else { // first time startup
		// there is no block 0 so leave that empty
		fmt.Printf("setting h=1\n")
		_, err = ff.offsetFile.Write(make([]byte, 8))
		if err != nil {
			return err
		}
		// do the same with the in-ram slice
		ff.offsets = make([]int64, 1)
		// start writing at block 1
		ff.currentHeight = 1
	}

	return nil
}

func (ff *flatFileState) writeUndoBlock(ub accumulator.UndoBlock) error {
	undoSize := ub.SerializeSize()
	buf := make([]byte, undoSize)

	// write the offset of current of undo block to offset file
	buf = buf[:8]
	ff.offsets = append(ff.offsets, ff.currentOffset)

	binary.BigEndian.PutUint64(buf, uint64(ff.currentOffset))
	_, err := ff.offsetFile.WriteAt(buf, int64(8*ub.Height))
	if err != nil {
		return err
	}

	// write to undo file
	_, err = ff.proofFile.WriteAt([]byte{0xaa, 0xff, 0xaa, 0xff}, ff.currentOffset)
	if err != nil {
		return err
	}

	//prefix with size of the undoblocks
	buf = buf[:4]
	binary.BigEndian.PutUint32(buf, uint32(undoSize))
	_, err = ff.proofFile.WriteAt(buf, ff.currentOffset+4)
	if err != nil {
		return err
	}

	// Serialize UndoBlock
	buf = buf[:0]
	bytesBuf := bytes.NewBuffer(buf)
	err = ub.Serialize(bytesBuf)
	if err != nil {
		return err
	}

	_, err = ff.proofFile.WriteAt(bytesBuf.Bytes(), ff.currentOffset+4+4)
	if err != nil {
		return err
	}

	ff.currentOffset = ff.currentOffset + int64(undoSize) + 8
	ff.currentHeight++

	ff.fileWait.Done()

	return nil
}

func (ff *flatFileState) writeProofBlock(ud btcacc.UData) error {
	// get the new block proof
	// put offset in ram
	// write to offset file so we can resume; offset file is only
	// read on startup and always incremented so we shouldn't need to seek

	// pre-allocated the needed buffer
	udSize := ud.SerializeSize()
	buf := make([]byte, udSize)

	// write write the offset of the current proof to the offset file
	buf = buf[:8]
	ff.offsets = append(ff.offsets, ff.currentOffset)

	binary.BigEndian.PutUint64(buf, uint64(ff.currentOffset))
	_, err := ff.offsetFile.WriteAt(buf, int64(8*ud.Height))
	if err != nil {
		return err
	}

	// write to proof file
	_, err = ff.proofFile.WriteAt([]byte{0xaa, 0xff, 0xaa, 0xff}, ff.currentOffset)
	if err != nil {
		return err
	}

	// prefix with size
	buf = buf[:4]
	binary.BigEndian.PutUint32(buf, uint32(udSize))
	// +4 to account for the 4 magic bytes
	_, err = ff.proofFile.WriteAt(buf, ff.currentOffset+4)
	if err != nil {
		return err
	}

	// Serialize proof
	buf = buf[:0]
	bytesBuf := bytes.NewBuffer(buf)
	err = ud.Serialize(bytesBuf)
	if err != nil {
		return err
	}

	// Write to the file
	// +4 +4 to account for the 4 magic bytes and the 4 size bytes
	_, err = ff.proofFile.WriteAt(bytesBuf.Bytes(), ff.currentOffset+4+4)
	if err != nil {
		return err
	}

	// 4B magic & 4B size comes first
	ff.currentOffset += int64(ud.SerializeSize()) + 8
	ff.currentHeight++

	ff.fileWait.Done()

	return nil
}

func (ff *flatFileState) writeTTLs(ttlRes ttlResultBlock) error {
	var ttlArr [4]byte
	var buffer [8]byte

	// write ttl offset to offsetfile
	binary.BigEndian.PutUint64(buffer[:], uint64(ff.currentOffset))
	_, err := ff.offsetFile.WriteAt(buffer[:], int64(8*ttlRes.Height))
	if err != nil {
		return err
	}

	// for all the TTLs, seek and overwrite the empty values there
	for _, c := range ttlRes.Created {
		// seek to the location of that txo's ttl value in the proof file

		binary.BigEndian.PutUint32(
			ttlArr[:], uint32(ttlRes.Height-c.createHeight))

		// write it's lifespan as a 4 byte int32 (bit of a waste as
		// 2 or 3 bytes would work)
		_, err = ff.proofFile.WriteAt(ttlArr[:],
			ff.offsets[c.createHeight]+int64(c.indexWithinBlock*4))
		if err != nil {
			return err
		}
	}

	// increment value of offset 4 bytes of each ttlRes Created
	ff.currentOffset = ff.currentOffset + int64(len(ttlRes.Created)*4)
	// increment height by 1
	ff.currentHeight = ff.currentHeight + 1
	ff.fileWait.Done()
	return nil
}
