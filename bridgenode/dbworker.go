package bridgenode

import (
	"encoding/binary"
	"fmt"
	"os"
	"sync"

	"github.com/mit-dci/utreexo/util"
	"github.com/syndtr/goleveldb/leveldb"
)

// DbWorker writes & reads/deletes everything to the db.
// It also generates TTLResultBlocks to send to the flat file worker
func DbWorker(
	dbWorkChan chan ttlRawBlock, ttlResultChan chan ttlResultBlock, lvdb *leveldb.DB, wg *sync.WaitGroup) {

	// open the offsetFile read-only
	offsetFile, err := os.Open(util.POffsetFilePath)
	if err != nil {
		panic(err)
	}

	val := make([]byte, 4)
	offsetBytes := make([]byte, 8)

	for {
		work := <-dbWorkChan
		var batch leveldb.Batch
		// build the batch for writing to levelDB.
		// Just outpoints to index within block
		for i, op := range work.newTxos {
			binary.BigEndian.PutUint32(val, uint32(i))
			batch.Put(op[:], val)
		}
		// write all the new utxos in the batch to the DB
		err := lvdb.Write(&batch, nil)
		if err != nil {
			fmt.Println(err.Error())
		}
		batch.Reset()

		inBlockIdxs := make([]uint32, len(work.spentTxos))
		// now read from the DB all the spent txos and find their
		// position within their creation block
		for i, op := range work.spentTxos {
			batch.Delete(op[:]) // add this outpoint for deletion
			idxBytes, err := lvdb.Get(op[:], nil)
			if err != nil {
				panic(err)
			}
			inBlockIdxs[i] = binary.BigEndian.Uint32(idxBytes)
		}
		// delete all the old outpoints
		err = lvdb.Write(&batch, nil)
		if err != nil {
			fmt.Println(err.Error())
		}

		// make sure that the offset file has been written to for this block
		// we might not be writing to things that recent but usually are, so
		// wait for the flatfile to be created before we start writing.
		// usually this will have already happened I think
		// stat, err := offsetFile.Stat()
		// if err != nil {
		// 	fmt.Printf("DbWorker error stating offsetfile\n")
		// 	panic(err)
		// }
		// for stat.Size() < work.blockHeight*8 {
		// 	stat, err := offsetFile.Stat()
		// }

		// open the proof file for writing.  Guess we have to fight with
		// proofWriterWorker() for this?  Or maybe we can give commands over the
		// channel to proofWriterWorker()

		// TODO ok yeah I think it's better to expand proofWriterWorker() to
		// absorb some of this data because it's got the proof file under its
		// control and it seems easier to let it be the only one writing to that

		// proofFile, err := os.OpenFile(
		// util.PFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		// if err != nil {
		// panic(err)
		// }

		// offsetFile, err := os.Open(util.POffsetFilePath)
		// if err != nil {
		// 	panic(err)
		// }

		// ssh is spent start height.  blockheight-ssh is the utxo duration
		// range through every spent txo here, look up the block they were
		// created in, seek to that block plus the index of that txo creation
		// then write the duration value
		for _, ssh := range work.spentStartHeights {

			// look up this utxo's creation block offset
			_, err := offsetFile.ReadAt(offsetBytes, int64(ssh)*8)
			if err != nil {
				panic(err)
			}
			// blockStartOffset = int64(binary.BigEndian.Uint64(offsetBytes))

			binary.Write(offsetFile, binary.BigEndian, work.blockHeight-ssh)

			// work.blockHeight-ssh
		}
		wg.Done()
	}
}
