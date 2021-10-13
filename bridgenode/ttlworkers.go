package bridgenode

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// BNRTTLSplit gets a block&rev and splits the input and output sides.  it
// sends the output side to the txid sorter, and the input side to the
// ttl lookup worker
func BNRTTLSpliter(
	bnrChan chan blockAndRev, ttlResultChan chan ttlResultBlock,
	utdir utreeDir) {

	txidFile, err := os.OpenFile(
		filepath.Join(utdir.TtlDir.base, "txidFile"),
		os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		panic(err)
	}

	txidOffsetFile, err := os.OpenFile(
		filepath.Join(utdir.TtlDir.base, "txidOffsetFile"),
		os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		panic(err)
	}

	startOffset, err := txidFile.Seek(0, 2)
	if err != nil {
		panic(err)
	}
	startOffset >>= 3 // divide by 8 to get the offset in miniTxids

	// seek to the end of the offset file as TxidSortWriterWorker will start
	// appending to it
	_, err = txidOffsetFile.Seek(0, 2)
	if err != nil {
		panic(err)
	}
	writeBlockChan := make(chan ttlWriteBlock, 10)
	lookupChan := make(chan ttlLookupBlock, 10)
	goChan := make(chan bool, 10)

	go TxidSortWriterWorker(
		writeBlockChan, goChan, startOffset, txidFile, txidOffsetFile)

	// TTLLookupWorker needs to send the final data to the flatFileWorker
	go TTLLookupWorker(
		lookupChan, ttlResultChan, goChan, txidFile, txidOffsetFile)

	for {
		bnr, open := <-bnrChan
		if !open {
			break
		}
		// these are the blocks where testnet messes up
		// if bnr.Height == 206421 || bnr.Height == 205955 {
		// fmt.Printf(bnr.toString())
		// }
		// var txoInBlock uint16
		var lub ttlLookupBlock
		var wb ttlWriteBlock
		var inskippos, inputInBlock uint32
		var outputInBlock uint16
		var keepSkippingInputs bool
		inskipMax := uint32(len(bnr.inSkipList))

		lub.destroyHeight = bnr.Height
		transactions := bnr.Blk.Transactions()

		wb.createHeight = bnr.Height
		wb.mTxids = make([]miniTx, len(transactions))
		// fmt.Printf("h %d inskip %v\n", bnr.Height, bnr.inSkipList)
		keepSkippingInputs = inskipMax > 0 // if none to skip, don't check

		// iterate through the transactions in a block
		for txInBlock, tx := range transactions {
			// add txid and skipped position in block
			wb.mTxids[txInBlock].txid = tx.Hash()
			wb.mTxids[txInBlock].startsAt = outputInBlock
			// first add all the outputs in this tx, then range through the
			// outputs and decrement them if they're on the skiplist
			mtx := tx.MsgTx()
			outputInBlock += uint16(len(mtx.TxOut))

			// for all the txins, throw that into the work as well; just a bunch of
			// outpoints
			for inputInTx, in := range tx.MsgTx().TxIn {
				// fmt.Printf("input in block %d ks %v sl %v isp %d\n",
				// inputInBlock, keepSkippingInputs, bnr.inSkipList, inskippos)
				if txInBlock == 0 {
					inputInBlock++
					inskippos++
					keepSkippingInputs = inskippos != inskipMax
					break // skip coinbase input
				}
				if keepSkippingInputs && bnr.inSkipList[inskippos] == inputInBlock {
					// fmt.Printf(" skipping tx %d input %d (%d in block)\n",
					// txInBlock, inputInTx, inputInBlock)
					inskippos++
					keepSkippingInputs = inskippos != inskipMax
					inputInBlock++
					continue
				}
				//make new miniIn
				mI := miniIn{idx: uint16(in.PreviousOutPoint.Index),
					createHeight: bnr.Rev.Txs[txInBlock-1].TxIn[inputInTx].Height}
				copy(mI.hashprefix[:], in.PreviousOutPoint.Hash[:6])
				// append outpoint to slice
				lub.spentTxos = append(lub.spentTxos, mI)
				inputInBlock++
			}
		}
		// done with block, send out split data to the two workers
		writeBlockChan <- wb
		lookupChan <- lub
	}
	close(writeBlockChan)
	close(lookupChan)
}

// TxidSortWriterWorker takes miniTxids in, sorts them, and writes them
// into a flat file (also writes the offsets files.  The offset file
// doesn't describe byte offsets, but rather 8 byte miniTxids
func TxidSortWriterWorker(
	tChan chan ttlWriteBlock, goChan chan bool, startOffset int64,
	miniTxidFile, txidOffsetFile io.Writer) {

	// sort then write.
	for {
		wb, open := <-tChan
		if !open {
			// fmt.Printf("TxidSortWriterWorker finished at height %d\n", wb.createHeight)
			break
		}
		// first write the current start offset, then increment it for next time
		// fmt.Printf("write h %d startOffset %d\t", height, startOffset)
		err := binary.Write(txidOffsetFile, binary.BigEndian, startOffset)
		if err != nil {
			panic(err)
		}
		startOffset += int64(len(wb.mTxids))
		sortTxids(wb.mTxids)
		err = wb.serialize(miniTxidFile)
		if err != nil {
			fmt.Printf("TTLWriteBlock write error: %s\n", err.Error())
		}
		goChan <- true // tell the TTLLookupWorker to start on the block just done
	}
}

// TODO: if the utxo is coinbase, don't have to look up position in block
// because you know it starts at 0.
// In fact could omit writing coinbase txids entirely?

// TTLLookupWorker gets miniInputs, looks up the txids, figures out
// how old the utxo lasted, and sends the resutls to writeTTLs via ttlResultChan

// Lookup happens after sorterWriter; the sorterWriter gives the OK to the
// TTL lookup worker after its done writing to its files
func TTLLookupWorker(
	lChan chan ttlLookupBlock, ttlResultChan chan ttlResultBlock, goChan chan bool,
	txidFile, txidOffsetFile *os.File) {
	var seekHeight int32
	var heightOffset, nextOffset int64
	var startOffsetBytes, nextOffsetBytes [8]byte

	for {
		<-goChan
		lub, open := <-lChan
		if !open {
			break
		}
		// build a TTL result block
		var resultBlock ttlResultBlock
		resultBlock.destroyHeight = lub.destroyHeight
		resultBlock.results = make([]ttlResult, len(lub.spentTxos))

		// sort the txins by utxo height; hopefully speeds up search
		sortMiniIns(lub.spentTxos)
		for i, stxo := range lub.spentTxos {
			// fmt.Printf("need txid %x from height %d\n", stxo.hashprefix, stxo.height)
			if stxo.createHeight != seekHeight { // height change, get byte offsets
				// subtract 1 from stxo height because this file starts at height 1
				_, err := txidOffsetFile.ReadAt(
					startOffsetBytes[:], int64(stxo.createHeight-1)*8)
				if err != nil {
					fmt.Printf("tried to read at txidoffset file byte %d  ",
						(stxo.createHeight-1)*8)
					panic(err)
				}

				heightOffset = int64(binary.BigEndian.Uint64(startOffsetBytes[:]))

				// TODO: make sure this is OK.  If we always have a
				// block after the one we're seeking this will not error.

				_, err = txidOffsetFile.ReadAt(
					nextOffsetBytes[:], int64(stxo.createHeight)*8)
				if err != nil {
					fmt.Printf("tried to read next at %d  ", stxo.createHeight*8)
					panic(err)
				}
				nextOffset = int64(binary.BigEndian.Uint64(nextOffsetBytes[:]))
				// if nextOffset==heightOffset{}
				if nextOffset < heightOffset {
					fmt.Printf("nextOffset %d < start %d byte %d\n",
						nextOffset, heightOffset, stxo.createHeight*8)
					panic("bad offset")
				}
				seekHeight = stxo.createHeight
			}
			if stxo.createHeight == resultBlock.destroyHeight {
				fmt.Printf("\tXXXXh %d stxo %d trying to write 0 TTL %x:%d.\n",
					resultBlock.destroyHeight, i, stxo.hashprefix, stxo.idx)
				if stxo.createHeight > 108 {
					panic("0 ttl")
				}
			}

			resultBlock.results[i].createHeight = stxo.createHeight
			// fmt.Printf("search for create height %d %x:%d from %d range %d\n",
			// stxo.createHeight, stxo.hashprefix, stxo.idx,
			// heightOffset, nextOffset-heightOffset)

			resultBlock.results[i].indexWithinBlock =
				binSearch(stxo, heightOffset, nextOffset, txidFile)

			// fmt.Printf("h %d stxo %x:%d writes ttl value %d to h %d idxinblk %d\n",
			// lub.destroyHeight, stxo.hashprefix, stxo.idx,
			// lub.destroyHeight-resultBlock.results[i].createHeight,
			// stxo.createHeight,
			// resultBlock.results[i].indexWithinBlock)

		}

		ttlResultChan <- resultBlock
	}

	err := txidFile.Close()
	if err != nil {
		panic(err)
	}
	err = txidOffsetFile.Close()
	if err != nil {
		panic(err)
	}
}

// actually start with a binary search, easier
func binSearch(mi miniIn,
	bottom, top int64, mtxFile io.ReaderAt) uint16 {
	// fmt.Printf("looking for %x blkstart/end %d/%d\n", mi.hashprefix, blkStart, blkEnd)

	var positionBytes [2]byte
	var pos int // position in transactions
	width := int(top - bottom)
	if width == 0 {
		pos = int(bottom)
	} else {
		pos = sort.Search(
			width, searchReaderFunc(int(bottom), mi.hashprefix, mtxFile))
		if pos >= width {
			fmt.Printf("WARNING can't find %x\n", mi.hashprefix)
			panic("failed txid search")
		}
	}
	// read positionBytes which tells utxo position from tx position
	n, err := mtxFile.ReadAt(positionBytes[:], int64((pos+int(bottom))*8)+6)
	if err != nil || n != 2 {
		panic(err)
	}
	// fmt.Printf("%x got position %d width %d, read bytes %x from pos %d\n",
	// mi.hashprefix, pos, width, positionBytes, (pos*8)+6)
	// fmt.Printf("found %x at pos %d, read %x\n",
	// mi.hashprefix, pos+int(bottom), positionBytes)
	// add to the index of the outpoint to get the position of the txo among
	// all the block's txos
	return binary.BigEndian.Uint16(positionBytes[:]) + mi.idx
}

// using sort.Search is inefficient because - well first it's binary not
// interpolation which works better here because its uniform hashes, but also
// because it's trying to find the lowest index or first instance of a target,
// but in this case we know the hashes are unique so we don't have keep going
// once we find it.
func searchReaderFunc(
	startPosition int, lookFor [6]byte, mtxFile io.ReaderAt) func(int) bool {
	return func(pos int) bool {
		var miniTxidBytes [6]byte
		mtxFile.ReadAt(miniTxidBytes[:], int64(pos+startPosition)*8)
		// fmt.Printf("looking for %x at pos %d idx %d byte position %d, found %x\n",
		// lookFor, pos, pos+startPosition, int64(pos+startPosition)*8, miniTxidBytes)
		return miniBytesToUint64(miniTxidBytes) >= miniBytesToUint64(lookFor)
	}
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

	_, _ = mtx.Seek(guessPos*8, 0)
	mtx.Read(guessMi.hashprefix[:])

	for {
	}
	return
}
