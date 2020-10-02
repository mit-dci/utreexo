package bridgenode

import (
	"encoding/binary"
	"fmt"
	"os"
	"sync"

	"github.com/btcsuite/btcd/wire"

	"github.com/mit-dci/utreexo/util"

	"github.com/syndtr/goleveldb/leveldb"
)

/*
Here's what it *did*:   (can delete after this is old)

genproof:

Get a block at current height
Look through txins
get utxo identifier (outpoint) and current block height
write outpoint: deathheight to db (current height is deathheight)

serve:

Get a block at current height
look through all the outputs and calculate outpoint
lookup outpoints in DB and get deathheight
subtract deathheight from current height, get duration for the new utxo

That's bad!  Because you're doing lots o fDB lookups when serving which is slow,
and you're keepting a big DB of every txo ever.

--------------------------------------------------

Here's what it *does* now:

genproof:

Get a block at current height:

look through txouts, put those in DB.  Also put their place in the block
db key: outpoint db value: txoinblock index (4 bytes)

also look through txins.  for each of them, look them up in the db,
getting the birthheight and which txo it is within that block.
lookup the block offset where the txo was created, seek to that block,
then seek the the 4 byte location where its duration is stored (starts as
unknown / 0 / -1 whatever) and overwrite it.

Also delete the db entry for the txin.
And yeah this is a utxo db so if we put more data here, then we don't
need the rev files... and if the rev files told which txout in the block it
was... we wouldn't need a db at all.

serve:

Get a block at current height
all the ttl values are right there in the block, nothing to do

That's better, do more work in genproofs since that only happens once and
serving can happen lots of times.


*/

// the thing we send to the dbWorker; a db batch and some ttl results
type ttlBlock struct {
	blockHeight       int32
	newTxos           [][36]byte
	spentTxos         [][36]byte
	spentStartHeights []int32
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

// WriteBlock sends off TTL-related data to dbWorker to be written to ttldb and
// looked up for insertion into the flat file
func WriteBlock(bnr BlockAndRev, batchan chan ttlBlock, wg *sync.WaitGroup) {
	var work ttlBlock
	work.blockHeight = bnr.Height

	// iterate through the transactions in a block
	for txInBlock, tx := range bnr.Blk.Transactions {
		txid := tx.TxHash()

		// for all the txouts, get their outpoint & index and throw that into
		// a db batch
		for txoInTx, _ := range tx.TxOut {
			work.newTxos = append(work.newTxos,
				util.OutpointToBytes(wire.NewOutPoint(&txid, uint32(txoInTx))))

			// for all the txins, throw that into the work as well; just a bunch of
			// outpoints

			for txinInTx, in := range tx.TxIn {
				if txInBlock == 0 {
					break // skip coinbase input
				}
				// append outpoint to slice
				work.spentTxos = append(work.spentTxos,
					util.OutpointToBytes(&in.PreviousOutPoint))
				// append start height to slice (get from rev data)
				work.spentStartHeights = append(work.spentStartHeights,
					bnr.Rev.Txs[txInBlock].TxIn[txinInTx].Height)
			}
		}
	}

	wg.Add(1)
	// send to dbworker to be written to ttldb asynchronously
	batchan <- work
}

// DbWorker writes everything to the db. It's it's own goroutine so it
// can work at the same time that the reads are happening
func DbWorker(
	dbWorkChan chan ttlBlock, lvdb *leveldb.DB, wg *sync.WaitGroup) {

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
