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
	heightOffsets         []int64
	proofFile, offsetFile *os.File
	finishedHeight        int32
	currentOffset         int64
	fileWait              *sync.WaitGroup
}

func flatFileWorkerProof(
	proofChan chan btcacc.UData,
	utreeDir utreeDir,
	fileWait *sync.WaitGroup) {

	var pf flatFileState
	var err error

	pf.offsetFile, err = os.OpenFile(
		utreeDir.ProofDir.pOffsetFile, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		panic(err)
	}

	pf.proofFile, err = os.OpenFile(
		utreeDir.ProofDir.pFile, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}

	pf.fileWait = fileWait

	err = pf.ffInit()
	if err != nil {
		panic(err)
	}

	for {
		ud := <-proofChan
		err = pf.writeProofBlock(ud)
		if err != nil {
			panic(err)
		}
	}
}

func flatFileWorkerUndo(
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

func flatFileWorkerTTL(
	ttlResultChan chan ttlResultBlock,
	numLeavesChan chan int,
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
		// expand TTL file by 4 byte for every utxo in this block
		size := <-numLeavesChan
		err = tf.proofFile.Truncate(tf.currentOffset + int64(size*4))
		if err != nil {
			panic(err)
		}
		fmt.Printf("h %d %d utxos truncated from %d to %d\n",
			len(tf.heightOffsets), size,
			tf.currentOffset, tf.currentOffset+int64(size*4))

		// get the TTL resutls for this block and write to previously
		// allocated locations
		ttlRes := <-ttlResultChan
		err = tf.writeTTLs(ttlRes)
		if err != nil {
			panic(err)
		}
		// append tf offsets after writing ttl data
		tf.heightOffsets = append(tf.heightOffsets, tf.currentOffset)
		// increment currentoffset value
		tf.currentOffset = tf.currentOffset + int64(size*4)
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
		savedHeight := int32(offsetFileSize/8) - 1
		// TODO I'm not really sure why theres a -1 there
		// seek back to the file start / block "0"
		_, err := ff.offsetFile.Seek(0, 0)
		if err != nil {
			return err
		}
		ff.heightOffsets = make([]int64, savedHeight)
		for ff.finishedHeight < savedHeight {
			err = binary.Read(ff.offsetFile, binary.BigEndian, &ff.currentOffset)
			if err != nil {
				fmt.Printf("couldn't populate in-ram offsets on startup")
				return err
			}
			ff.heightOffsets[ff.finishedHeight] = ff.currentOffset
			ff.finishedHeight++
		}

		// set currentOffset to the end of the proof file
		ff.currentOffset, err = ff.proofFile.Seek(0, 2)
		if err != nil {
			return err
		}

	} else { // first time startup
		// there is no block 0 so leave that empty
		// fmt.Printf("setting h=1\n")
		_, err = ff.offsetFile.Write(make([]byte, 8))
		if err != nil {
			return err
		}
		// do the same with the in-ram slice
		ff.heightOffsets = make([]int64, 1)
		// does nothing.  We're *finished* writing block 0
		ff.finishedHeight = 0
	}

	return nil
}

func (uf *flatFileState) writeUndoBlock(ub accumulator.UndoBlock) error {
	undoSize := ub.SerializeSize()
	buf := make([]byte, undoSize)

	// write the offset of current of undo block to offset file
	buf = buf[:8]
	uf.heightOffsets = append(uf.heightOffsets, uf.currentOffset)

	binary.BigEndian.PutUint64(buf, uint64(uf.currentOffset))
	_, err := uf.offsetFile.WriteAt(buf, int64(8*ub.Height))
	if err != nil {
		return err
	}

	// write to undo file
	_, err = uf.proofFile.WriteAt([]byte{0xaa, 0xff, 0xaa, 0xff}, uf.currentOffset)
	if err != nil {
		return err
	}

	//prefix with size of the undoblocks
	buf = buf[:4]
	binary.BigEndian.PutUint32(buf, uint32(undoSize))
	_, err = uf.proofFile.WriteAt(buf, uf.currentOffset+4)
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

	_, err = uf.proofFile.WriteAt(bytesBuf.Bytes(), uf.currentOffset+4+4)
	if err != nil {
		return err
	}

	uf.currentOffset = uf.currentOffset + int64(undoSize) + 8
	uf.finishedHeight++

	uf.fileWait.Done()

	return nil
}

func (pf *flatFileState) writeProofBlock(ud btcacc.UData) error {
	// fmt.Printf("udata height %d flat file height %d\n",
	// ud.Height, ff.finishedHeight)

	// get the new block proof
	// put offset in ram
	// write to offset file so we can resume; offset file is only
	// read on startup and always incremented so we shouldn't need to seek

	// pre-allocated the needed buffer
	udSize := ud.SerializeSize()
	lilBuf := make([]byte, udSize)

	// write write the offset of the current proof to the offset file
	lilBuf = lilBuf[:8]
	pf.heightOffsets = append(pf.heightOffsets, pf.currentOffset)

	binary.BigEndian.PutUint64(lilBuf, uint64(pf.currentOffset))
	_, err := pf.offsetFile.WriteAt(lilBuf, int64(8*ud.Height))
	if err != nil {
		return err
	}

	// write to proof file
	_, err = pf.proofFile.WriteAt([]byte{0xaa, 0xff, 0xaa, 0xff}, pf.currentOffset)
	if err != nil {
		return err
	}

	// prefix with size
	lilBuf = lilBuf[:4]
	binary.BigEndian.PutUint32(lilBuf, uint32(udSize))
	// +4 to account for the 4 magic bytes
	_, err = pf.proofFile.WriteAt(lilBuf, pf.currentOffset+4)
	if err != nil {
		return err
	}

	// Serialize proof
	lilBuf = lilBuf[:0]
	bigBuf := bytes.NewBuffer(lilBuf)
	err = ud.Serialize(bigBuf)
	if err != nil {
		return err
	}

	// Write to the file
	// +4 +4 to account for the 4 magic bytes and the 4 size bytes
	_, err = pf.proofFile.WriteAt(bigBuf.Bytes(), pf.currentOffset+4+4)
	if err != nil {
		return err
	}

	// 4B magic & 4B size comes first
	pf.currentOffset += int64(ud.SerializeSize()) + 8
	pf.finishedHeight++

	if ud.Height != pf.finishedHeight {
		fmt.Printf("WARNING udata height %d flat file height %d\n",
			ud.Height, pf.finishedHeight)
	}

	pf.fileWait.Done()
	return nil
}

func (tf *flatFileState) writeTTLs(ttlRes ttlResultBlock) error {

	var ttlArr, readEmpty, expectedEmpty [4]byte

	// for all the TTLs, seek and overwrite the empty values there
	for _, c := range ttlRes.results {
		if c.createHeight >= int32(len(tf.heightOffsets)) {
			return fmt.Errorf("utxo created h %d idx in block %d destroyed h %d"+
				" but max h %d cur h %d", c.createHeight, c.indexWithinBlock,
				ttlRes.destroyHeight, len(tf.heightOffsets), tf.finishedHeight)
		}

		binary.BigEndian.PutUint32(
			ttlArr[:], uint32(ttlRes.destroyHeight-c.createHeight))

		// calculate location of that txo's ttl value in the proof file:
		// write it's lifespan as a 4 byte int32 (bit of a waste as
		// 2 or 3 bytes would work)
		loc := tf.heightOffsets[c.createHeight] + int64(c.indexWithinBlock)*4

		// first, read the data there to make sure it's empty.
		// If there's something already there, we messed up & should panic.
		// TODO once everything works great can remove this

		n, err := tf.proofFile.ReadAt(readEmpty[:], loc)
		if n != 4 && err != nil {
			fmt.Printf("ttl destroyH %d createH %d idxinblock %d\n",
				ttlRes.destroyHeight, c.createHeight, c.indexWithinBlock)
			fmt.Printf("want to read byte %d = hO[%d]=%d + %d * 4\n",
				loc, c.createHeight,
				tf.heightOffsets[c.createHeight], c.indexWithinBlock)
			s, _ := tf.proofFile.Stat()
			return fmt.Errorf("proofFile.ReadAt %d size %d %s",
				loc, s.Size(), err.Error())
		}

		if readEmpty != expectedEmpty {
			return fmt.Errorf("writeTTLs Wanted to overwrite byte %d with %x "+
				"but %x was already there. desth %d createh %d idxinblk %d",
				loc, ttlArr, readEmpty, ttlRes.destroyHeight,
				c.createHeight, c.indexWithinBlock)
		}

		// fmt.Printf("  writeTTLs overwrite byte %d with %x "+
		// "desth %d createh %d idxinblk %d\n",
		// loc, ttlArr, ttlRes.destroyHeight, c.createHeight, c.indexWithinBlock)

		// fmt.Printf("overwriting %x with %x\t", readEmpty, ttlArr)
		_, err = tf.proofFile.WriteAt(ttlArr[:], loc)
		if err != nil {
			return fmt.Errorf("proofFile.WriteAt %d %s", loc, err.Error())
		}

	}

	// increment value of offset 4 bytes of each ttlRes Created
	tf.currentOffset = tf.currentOffset + int64(len(ttlRes.results)*4)
	// increment height by 1
	tf.finishedHeight = tf.finishedHeight + 1
	tf.fileWait.Done()
	return nil
}
