package bridgenode

import (
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/syndtr/goleveldb/leveldb"
)

// DbWorker writes & reads/deletes everything to the db.
// It also generates TTLResultBlocks to send to the flat file worker
func DbWorker(
	dbWorkChan chan ttlRawBlock, ttlResultChan chan ttlResultBlock,
	lvdb *leveldb.DB, wg *sync.WaitGroup) {

	val := make([]byte, 4)

	for {
		dbBlock := <-dbWorkChan
		var batch leveldb.Batch
		// build the batch for writing to levelDB.
		// Just outpoints to index within block
		for i, op := range dbBlock.newTxos {
			binary.BigEndian.PutUint32(val, uint32(i))
			batch.Put(op[:], val)
		}
		// write all the new utxos in the batch to the DB
		err := lvdb.Write(&batch, nil)
		if err != nil {
			fmt.Println(err.Error())
		}
		batch.Reset()

		var trb ttlResultBlock

		trb.Height = dbBlock.blockHeight
		trb.Created = make([]txoStart, len(dbBlock.spentTxos))

		// now read from the DB all the spent txos and find their
		// position within their creation block
		for i, op := range dbBlock.spentTxos {
			batch.Delete(op[:]) // add this outpoint for deletion
			idxBytes, err := lvdb.Get(op[:], nil)
			if err != nil {
				fmt.Printf("can't find %x in db\n", op)
				panic(err)
			}

			// skip txos that live 0 blocks as they'll be deduped out of the
			// proofs anyway
			if dbBlock.spentStartHeights[i] != dbBlock.blockHeight {
				trb.Created[i].indexWithinBlock = binary.BigEndian.Uint32(idxBytes)
				trb.Created[i].createHeight = dbBlock.spentStartHeights[i]
			}
		}
		// send to flat ttl writer
		ttlResultChan <- trb
		err = lvdb.Write(&batch, nil) // actually delete everything
		if err != nil {
			fmt.Println(err.Error())
		}

		wg.Done()
	}
}
