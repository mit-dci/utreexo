package bridgenode

import (
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/mit-dci/utreexo/util"
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
func WriteBlock(bnr BlockAndRev,
	batchan chan *leveldb.Batch, wg *sync.WaitGroup) {

	blockBatch := new(leveldb.Batch)

	// iterate through the transactions in a block
	for blockindex, tx := range bnr.Blk.Transactions {
		// iterate through individual inputs in a transaction
		for _, in := range tx.TxIn {
			if blockindex > 0 { // skip coinbase "spend"
				opString := in.PreviousOutPoint.String()
				h := util.HashFromString(opString)
				heightBytes := make([]byte, 4)
				binary.BigEndian.PutUint32(
					heightBytes,
					uint32(bnr.Height+1), // why +1??
				)
				// TODO ^^^^^ yeah why
				blockBatch.Put(h[:], heightBytes)
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
