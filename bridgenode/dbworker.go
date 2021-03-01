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
		lub.destroyHeight = bnr.Height
		transactions := bnr.Blk.Transactions()
		miniTxSlice := make([]miniTx, len(transactions))
		// iterate through the transactions in a block
		for txInBlock, tx := range transactions {
			var txoInBlock uint16
			// ignore skiplists for now?
			// TODO use skiplists.  Saves space.

			// add all txids
			miniTxSlice[txInBlock].txid = tx.Hash()
			miniTxSlice[txInBlock].startsAt = txoInBlock
			txoInBlock += uint16(len(tx.Txos()))

			// for all the txins, throw that into the work as well; just a bunch of
			// outpoints
			for inputInTx, in := range tx.MsgTx().TxIn {
				if txInBlock == 0 {
					// inputInBlock += uint32(len(tx.MsgTx().TxIn))
					break // skip coinbase input
				}
				//make new miniIn
				mI := miniIn{idx: uint16(in.PreviousOutPoint.Index),
					height: bnr.Rev.Txs[txInBlock-1].TxIn[inputInTx].Height}
				copy(mI.hashprefix[:], in.PreviousOutPoint.Hash[:6])
				// append outpoint to slice
				lub.spentTxos = append(lub.spentTxos, mI)
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
	var heightOffset, nextOffset int64

	for {
		lub := <-lChan

		// build a TTL result block
		var resultBlock ttlResultBlock
		resultBlock.destroyHeight = lub.destroyHeight
		resultBlock.results = make([]ttlResult, len(lub.spentTxos))

		// sort the txins by utxo height; hopefully speeds up search

		sortMiniIns(lub.spentTxos)
		for i, stxo := range lub.spentTxos {
			if stxo.height != seekHeight { // height change, get byte offsets
				offsets.Seek(int64(stxo.height*8), 0) // offsets are 8bytes each
				binary.Read(offsets, binary.BigEndian, &heightOffset)
				// TODO: make sure this is OK.  If we always have a
				// block after the one we're seeking this will not error.
				binary.Read(offsets, binary.BigEndian, &nextOffset)
				seekHeight = stxo.height
			}

			resultBlock.results[i].createHeight = stxo.height
			resultBlock.results[i].indexWithinBlock =
				binSearch(stxo, heightOffset, nextOffset, txidFile)

		}
	}
}

// actually start with a binary search, easier
func binSearch(mi miniIn,
	blkStart, blkEnd int64, mtx io.ReadSeeker) (txoPosInBlock uint16) {

	var guessMi miniIn

	top, bottom := blkEnd, blkStart
	// start in the middle
	guessPos := (top + bottom) / 2
	_, _ = mtx.Seek(guessPos*16, 0)
	mtx.Read(guessMi.hashprefix[:])

	for guessMi.hashprefix != mi.hashprefix {
		if guessMi.hashToUint64() > mi.hashToUint64() { // too high, lower top
			top = guessPos
		} else { // must be too low (not equal), raise bottom
			bottom = guessPos
		}
		guessPos = (top + bottom) / 2   // pick a position halfway in the range
		_, _ = mtx.Seek(guessPos*16, 0) // seek & read
		mtx.Read(guessMi.hashprefix[:])
	}
	// found it, read the next 2 bytes to get starting point of tx
	binary.Read(mtx, binary.BigEndian, &txoPosInBlock)
	// add to the index of the outpoint to get the position of the txo among
	// all the block's txos
	txoPosInBlock += mi.idx
	return
}

// interpSearch performs an interpolation search
// give it a miniInput, the start and end positions of the block creating it,
// as well as the block file, and it will return the position within the block
// of that output.
// blkStart and blkEnd are positions, not byte offsets; for byte offsets
// multiply by 16
func interpolationSearch(mi miniIn,
	blkStart, blkEnd int64, mtx io.ReadSeeker) (txoPosInBlock uint16) {

	var guessMi miniIn

	topPos, bottomPos := blkEnd, blkStart
	topVal := uint64(0x0000ffffffffffff)
	bottomVal := uint64(0)

	// guess where it is based on ends
	guessPos := int64(guessMi.hashToUint64()/(topVal-bottomVal)) * (topPos - bottomPos)
	// nah that won't work.  Maybe need floats or something

	_, _ = mtx.Seek(guessPos*16, 0)
	mtx.Read(guessMi.hashprefix[:])

	for {
	}
	return
}
