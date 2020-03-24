package ttl

import (
	"fmt"
	"sync"

	"github.com/btcsuite/btcutil"
	"github.com/mit-dci/utreexo/cmd/util"
	"github.com/syndtr/goleveldb/leveldb"
)

//writeBlock sends off ttl info to dbWorker to be written to ttldb
func WriteBlock(tx []*btcutil.Tx, tipnum int32,
	batchan chan *leveldb.Batch, wg *sync.WaitGroup) error {

	blockBatch := new(leveldb.Batch)

	for blockindex, tx := range tx {
		for _, in := range tx.MsgTx().TxIn {
			if blockindex > 0 { // skip coinbase "spend"
				//hashing because blockbatch wants a byte slice
				//TODO Maybe don't convert to a string?
				//Perhaps converting to bytes can work?
				opString := in.PreviousOutPoint.String()
				h := util.HashFromString(opString)
				blockBatch.Put(h[:], util.U32tB(uint32(tipnum)))
			}
		}
	}

	//fmt.Printf("--- sending off %d dels at tipnum %d\n", batch.Len(), tipnum)
	wg.Add(1)

	//sent to dbworker to be written to ttldb asynchronously
	batchan <- blockBatch

	return nil
}

// dbWorker writes everything to the db. It's it's own goroutine so it
// can work at the same time that the reads are happening
// receives from writeBlock
func DbWorker(
	bChan chan *leveldb.Batch, lvdb *leveldb.DB, wg *sync.WaitGroup) {

	for {
		b := <-bChan
		//		fmt.Printf("--- writing batch %d dels\n", b.Len())
		err := lvdb.Write(b, nil)
		if err != nil {
			fmt.Println(err.Error())
		}
		//		fmt.Printf("wrote %d deletions to leveldb\n", b.Len())
		wg.Done()
	}
}
