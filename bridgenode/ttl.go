package bridgenode

import (
	"encoding/binary"
	"fmt"
	"os"
	"sync"

	"github.com/mit-dci/utreexo/util"

	"github.com/syndtr/goleveldb/leveldb"
)

// the thing we send to the dbWorker; a db batch and some ttl results
type dbWork struct {
	blockHeight int32
	ttldata     []TTLResult
	batch       *leveldb.Batch
}

// a TTLResult is the TTL data we learn once a txo is spent & it's lifetime
// can be written to its creation ublock
// When writing to the flat file, you want to seek to StartHeight, skip to
// the TxoNumInBlock'th entry, and write Duration
type TTLResult struct {
	StartHeight, Duration int32
	TxoNumInBlock         uint32
	//  ^^^ could be uint16 since there can't be 65K txos in a block
}

// WriteBlock sends off ttl info to dbWorker to be written to ttldb
func WriteBlock(bnr BlockAndRev,
	batchan chan dbWork, wg *sync.WaitGroup) {

	blockBatch := new(leveldb.Batch)
	var work dbWork
	work.blockHeight = bnr.Height
	// write key:value to the ttl db; key is 8 bytes blockheight, txoutnumber
	var txoInBlock uint32
	// iterate through the transactions in a block
	for blockindex, tx := range bnr.Blk.Transactions {
		// iterate through individual inputs in a transaction
		for _, in := range tx.TxIn {
			if blockindex > 0 { // skip coinbase "spend"
				heightBytes := make([]byte, 4)
				binary.BigEndian.PutUint32(
					heightBytes,
					uint32(bnr.Height+1), // why +1??
				)
				// TODO ^^^^^ yeah why
				blockBatch.Put(
					util.OutpointToBytes(in.PreviousOutPoint), heightBytes)
			}
		}
		for _, out := range tx.TxOut {

			var result TTLResult

			// look up start height for this txin in db
			result.StartHeight = 1 // from db
			result.Duration = bnr.Height - result.StartHeight
			result.TxoNumInBlock = txoInBlock

			work.ttldata = append(work.ttldata)

			txoInBlock++
		}
	}

	wg.Add(1)

	work.batch = blockBatch
	// send to dbworker to be written to ttldb asynchronously
	batchan <- work
}

// DbWorker writes everything to the db. It's it's own goroutine so it
// can work at the same time that the reads are happening
func DbWorker(
	dbWorkChan chan dbWork, lvdb *leveldb.DB, wg *sync.WaitGroup) {

	// open the offsetFile read-only
	offsetFile, err := os.Open(util.POffsetFilePath)
	if err != nil {
		panic(err)
	}

	for {
		work := <-dbWorkChan
		err := lvdb.Write(b, nil)
		if err != nil {
			fmt.Println(err.Error())
		}
		ttlRes := <-ttlChan
		// make sure that the offset file has been written to for this block
		stat, err := offsetFile.Stat()
		// for stat.Size() <

		if err != nil {
			fmt.Printf("DbWorker error stating offsetfile\n")
			panic(err)
		}
		for _, result := range ttlRes {
			blockStartLoc, err := offsetFile.Seek(int64(result.StartHeight)*8, 0)
		}

		wg.Done()
	}
}
