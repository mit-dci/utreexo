package bridgenode

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sync"
)

// BNRTTLSplit gets a block&rev and splits the input and output sides.  it
// sends the output side to the txid sorter, and the input side to the
// ttl lookup worker
func BNRTTLSpliter(bnrChan chan BlockAndRev, wg *sync.WaitGroup) {

	txidFile, err := os.OpenFile(
		"/dev/shm/txidFile", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		panic(err)
	}
	txidOffsetFile, err := os.OpenFile(
		"/dev/shm/txidOffsetFile", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		panic(err)
	}

	miniTxidChan := make(chan []miniTx, 10)
	lookupChan := make(chan ttlLookupBlock, 10)

	go TxidSortWriterWorker(miniTxidChan, txidFile, txidOffsetFile)

	// TTLLookupWorker needs to send the final data to the flatFileWorker
	go TTLLookupWorker(lookupChan, txidFile, txidOffsetFile)

	for {
		bnr := <-bnrChan

		var lub ttlLookupBlock
		lub.blockHeight = bnr.Height
		transactions := bnr.Blk.Transactions()
		miniTxSlice := make([]miniTx, len(transactions))
		// iterate through the transactions in a block
		for txInBlock, tx := range transactions {
			// ignore skiplists for now?
			// TODO use skiplists.  Saves space.

			// add all txids
			miniTxSlice[txInBlock].txid = tx.Hash()
			miniTxSlice[txInBlock].numOuts = len(tx.Txos())

			// for all the txins, throw that into the work as well; just a bunch of
			// outpoints
			for inputInTx, in := range tx.MsgTx().TxIn {
				if txInBlock == 0 {
					// inputInBlock += uint32(len(tx.MsgTx().TxIn))
					break // skip coinbase input
				}
				// append outpoint to slice
				lub.spentTxos = append(lub.spentTxos,
					miniIn{
						op:     in.PreviousOutPoint,
						height: bnr.Rev.Txs[txInBlock-1].TxIn[inputInTx].Height})
			}
		}
		// done with block, send out split data to the two workers
		miniTxidChan <- miniTxSlice
		lookupChan <- lub
	}
}

func TxidSortWriterWorker(tChan chan []miniTx, mtxs, offsets io.Writer) {
	var startOffset int64 // starting byte offset of current block
	// sort then write.
	for {
		miniTxSlice := <-tChan
		// first write the current start offset, then increment it for next time
		err := binary.Write(offsets, binary.BigEndian, startOffset)
		if err != nil {
			panic(err)
		}
		startOffset += int64(len(miniTxSlice) * 16)

		sortTxids(miniTxSlice)
		for _, mt := range miniTxSlice {
			err := mt.serialize(mtxs)
			if err != nil {
				fmt.Printf("miniTx write error: %s\n", err.Error())
			}
		}
	}
}

func TTLLookupWorker(lChan chan ttlLookupBlock, txidFile, offsets io.ReadSeeker) {
	var seekHeight int32
	var heightOffset, nextOffset, blockLen int64
	// do an interpolation search
	for {
		lub := <-lChan
		// sort the txins by utxo height; should give better caching hopefuly
		sortMiniIns(lub.spentTxos)
		for _, stxo := range lub.spentTxos {
			if stxo.height != seekHeight {
				offsets.Seek(int64(stxo.height*8), 0) // offsets are 8bytes each
				binary.Read(offsets, binary.BigEndian, &heightOffset)
				// TODO: make sure this is OK.  If we always have a
				// block after the one we're seeking this will not error.
				binary.Read(offsets, binary.BigEndian, &nextOffset)
				blockLen = nextOffset - heightOffset
				seekHeight = stxo.height
			}
			blockLen++ // just to compile
			// do interpolation search here
		}
	}
}
