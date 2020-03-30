package ttl

import (
	"fmt"
	"sync"

	"github.com/mit-dci/utreexo/cmd/util"
	"github.com/syndtr/goleveldb/leveldb"
)

// DeathInfo is needed to asynchornously read from the leveldb
// Insures that the TxOs are in order
type DeathInfo struct {
	// DeathHeight is where the TxOs are spent
	// TxPos is the index within the TX
	DeathHeight, TxPos int32
	Txid               [32]byte
}

// WriteBlock sends off ttl info to dbWorker to be written to ttldb
func WriteBlock(txs util.BlockToWrite,
	batchan chan *leveldb.Batch, wg *sync.WaitGroup) {

	blockBatch := new(leveldb.Batch)

	// iterate through the transactions in a block
	for blockindex, tx := range txs.Txs {
		// iterate through individual inputs in a transaction
		for _, in := range tx.MsgTx().TxIn {
			if blockindex > 0 { // skip coinbase "spend"
				opString := in.PreviousOutPoint.String()
				h := util.HashFromString(opString)
				blockBatch.Put(h[:], util.U32tB(uint32(txs.Height+1)))
			}
		}
	}
	wg.Add(1)

	// send to dbworker to be written to ttldb asynchronously
	batchan <- blockBatch
}

// DbWorker writes everything to the db. It's it's own goroutine so it
// can work at the same time that the reads are happening
func DbWorker(
	bChan chan *leveldb.Batch, lvdb *leveldb.DB, wg *sync.WaitGroup) {

	for {
		b := <-bChan
		err := lvdb.Write(b, nil)
		if err != nil {
			fmt.Println(err.Error())
		}
		wg.Done()
	}
}
