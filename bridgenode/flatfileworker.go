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
func flatFileWorker(
	proofChan chan []byte,
	ttlResultChan chan ttlResultBlock,
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

	_, err = offsetFile.Seek(0, 2)
	if err != nil {
		panic(err)
	}

	proofFileLocation, err := proofFile.Seek(0, 2)
	if err != nil {
		panic(err)
	}

	proofWriteHeight := int32(proofFileLocation / 8)

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
		select {
		// if there's a proof waiting, write that first
		case pbytes := <-proofChan:
			// write to offset file first
			offsetFile.Seek(int64(proofWriteHeight)*8, 0)
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

			// then write the proof
			written, err := proofFile.Write(pbytes)
			if err != nil {
				fmt.Printf(err.Error())
				return
			}
			proofFileLocation += int64(written) + 8
			proofWriteHeight++
			fileWait.Done()
		}
		// separate select blocks, so that you try doing 1:1 proof & ttl
		// to prevent one buffer from filling up
		select {
		case ttlRes := <-ttlResultChan:

			if ttlRes.Height > proofWriteHeight {
				ttlResultChan <- ttlRes
				continue
				// this is weird and results in it being out of order, but...
				// should be OK!  Also if the buffer fills we're deadlocked here, since
				// this thread is the only thing pulling stuff out of this buffer.
				// This shouldn't happen since we check both buffers every loop
			}
			// write ttl results to disk
			for _, t := range ttlRes.Created {
				fmt.Printf("write ttl block %d txo %d lifespan %d\n",
					t.TxBlockHeight, t.IndexWithinBlock, ttlRes.Height-t.TxBlockHeight)

				// lookup block start point

			}

		}
	}
}
