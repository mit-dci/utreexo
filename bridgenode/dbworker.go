package bridgenode

import (
	"bytes"
	"io"
	"sync"
)

// BNRTTLSplit gets a block&rev and splits the input and output sides.  it
// sends the output side to the txid sorter, and the input side to the
// ttl lookup worker
func BNRTTLSpliter(bnrChan chan BlockAndRev, wg *sync.WaitGroup) {

	var sharedBuf bytes.Buffer
	miniTxidChan := make(chan []miniTx, 10)
	lookupChan := make(chan ttlLookupBlock, 10)

	go TxidSortWriterWorker(miniTxidChan, &sharedBuf)
	go TTLLookupWorker(lookupChan, &sharedBuf)

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

func TxidSortWriterWorker(tChan chan []miniTx, w io.Writer) {

	for {
		miniTxSlice := <-tChan
		sortTxids(miniTxSlice)

	}

	//sortTxids(miniTxSlice)
}

func TTLLookupWorker(lChan chan ttlLookupBlock, r io.Reader) {

}
