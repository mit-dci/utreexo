package ibdsim

import (
	"fmt"
	"os"
	"sync"

	"github.com/mit-dci/lit/wire"
	"github.com/mit-dci/utreexo/cmd/utils"
	"github.com/syndtr/goleveldb/leveldb"
)

//writeBlock sends off ttl info to dbWorker to be written to ttldb
func writeBlock(tx []*wire.MsgTx, tipnum int, tipFile *os.File,
	batchan chan *leveldb.Batch, wg *sync.WaitGroup) error {

	blockBatch := new(leveldb.Batch)

	for blockindex, tx := range tx {
		for _, in := range tx.TxIn {
			if blockindex > 0 { // skip coinbase "spend"
				//hashing because blockbatch wants a byte slice
				//TODO Maybe don't convert to a string?
				//Perhaps converting to bytes can work?
				opString := in.PreviousOutPoint.String()
				h := simutil.HashFromString(opString)
				blockBatch.Put(h[:], simutil.U32tB(uint32(tipnum)))
			}
		}
	}

	//fmt.Printf("--- sending off %d dels at tipnum %d\n", batch.Len(), tipnum)
	wg.Add(1)

	//sent to dbworker to be written to ttldb asynchronously
	batchan <- blockBatch

	//write to the .txos file
	_, err := tipFile.WriteAt(simutil.U32tB(uint32(tipnum)), 0)
	if err != nil {
		panic(err)
	}

	return nil
}

// dbWorker writes everything to the db. It's it's own goroutine so it
// can work at the same time that the reads are happening
// receives from writeBlock
func dbWorker(
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
