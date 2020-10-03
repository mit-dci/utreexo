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
*/

// pFileWorker takes in blockproof and height information from the channel
// and writes to disk. MUST NOT have more than one worker as the proofs need to be
// in order
func flatFileWriter(
	proofChan chan []byte, fileWait *sync.WaitGroup) {

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

	_, err = offsetFile.Seek(0, 2)
	if err != nil {
		panic(err)
	}

	proofFileLocation, err := proofFile.Seek(0, 2)
	if err != nil {
		panic(err)
	}

	// Grab either proof bytes and write em to offset / proof file, OR, get a whole
	// batch of

	for {

		pbytes := <-proofChan
		// write to offset file first
		err = binary.Write(offsetFile, binary.BigEndian, proofFileLocation)
		if err != nil {
			fmt.Printf(err.Error())
			return
		}

		// write to proof file
		// first write big endian proof size int64
		err = binary.Write(proofFile, binary.BigEndian, int64(len(pbytes)))
		if err != nil {
			fmt.Printf(err.Error())
			return
		}
		proofFileLocation += 8

		// then write the proof
		written, err := proofFile.Write(pbytes)
		if err != nil {
			fmt.Printf(err.Error())
			return
		}
		proofFileLocation += int64(written)

		fileWait.Done()
	}
}
