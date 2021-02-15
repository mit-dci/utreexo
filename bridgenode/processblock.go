package bridgenode

import (
	"fmt"

	"github.com/btcsuite/btcd/wire"
	"github.com/mit-dci/utreexo/btcacc"
	uwire "github.com/mit-dci/utreexo/wire"

	"github.com/mit-dci/utreexo/accumulator"
	"github.com/mit-dci/utreexo/util"
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

/*
general ttl flow here: block n rev read from disk.  Sent to block processing, which
turns it into a ttlRawBlock.  That gets sent to the DB worker, which does db io, and
turns the ttlRawBlock into a TtlResultBlock.  The TtlResultBlock then gets sent to the
flatfile worker which writes the ttl data to the right places within the flat file.
(the flat file worker is also getting proof data from the other processing which
has the accumulator, and it should only write ttl result blocks after it has already
processed the proof block.

Hopefully there are no timing / concurrency conflicts due to these things happening
on different threads.  I think as long as the flat file worker holds off on any
ttl blocks that come in too soon it should be OK; the db worker and everything else is
going sequentially and has buffers
*/

// the data from a block about txo creation and deletion for TTL calculation
// this will be sent to the DB
type ttlRawBlock struct {
	blockHeight       int32      // height of this block in the chain
	newTxos           [][36]byte // serialized outpoint for every output
	spentTxos         [][36]byte // serialized outpoint for every input
	spentStartHeights []int32    // tied 1:1 to spentTxos
}

// a TTLResult is the TTL data we learn once a txo is spent & it's lifetime
// can be written to its creation ublock
// When writing to the flat file, you want to seek to StartHeight, skip to
// the IndexWithinBlock'th entry, and write Duration (height- txBlockHeight)

// all the ttl result data from a block, after checking with the DB
// to be written to the flat file
type ttlResultBlock struct {
	Height  int32      // height of the block that consumed all the utxos
	Created []txoStart // slice of txo creation info
}

type txoStart struct {
	createHeight     int32  // what block created the txo
	indexWithinBlock uint32 // index in that block where the txo is created
}

// blockToAddDel turns a block into add leaves and del leaves
func blockToAddDel(bnr BlockAndRev) (
	blockAdds []accumulator.Leaf, delLeaves []btcacc.LeafData, err error) {

	inskip, outskip := bnr.Blk.DedupeBlock()
	// fmt.Printf("inskip %v outskip %v\n", inskip, outskip)
	delLeaves, err = blockNRevToDelLeaves(bnr, inskip)
	if err != nil {
		return
	}

	// this is bridgenode, so don't need to deal with memorable leaves
	blockAdds = uwire.BlockToAddLeaves(bnr.Blk, nil, outskip, bnr.Height)

	return
}

// blockNRevToDelLeaves turns a block's inputs into delLeaves to be removed from the
// accumulator
func blockNRevToDelLeaves(bnr BlockAndRev, skiplist []uint32) (
	delLeaves []btcacc.LeafData, err error) {

	// make sure same number of txs and rev txs (minus coinbase)
	if len(bnr.Blk.Transactions())-1 != len(bnr.Rev.Txs) {
		err = fmt.Errorf("genDels block %d %d txs but %d rev txs",
			bnr.Height, len(bnr.Blk.Transactions()), len(bnr.Rev.Txs))
		return
	}

	var blockInIdx uint32
	for txinblock, tx := range bnr.Blk.Transactions() {
		if txinblock == 0 {
			blockInIdx++ // coinbase tx always has 1 input
			continue
		}
		txinblock--
		// make sure there's the same number of txins
		if len(tx.MsgTx().TxIn) != len(bnr.Rev.Txs[txinblock].TxIn) {
			err = fmt.Errorf("genDels block %d tx %d has %d inputs but %d rev entries",
				bnr.Height, txinblock+1,
				len(tx.MsgTx().TxIn), len(bnr.Rev.Txs[txinblock].TxIn))
			return
		}
		// loop through inputs
		for i, txin := range tx.MsgTx().TxIn {
			// check if on skiplist.  If so, don't make leaf
			if len(skiplist) > 0 && skiplist[0] == blockInIdx {
				// fmt.Printf("skip %s\n", txin.PreviousOutPoint.String())
				skiplist = skiplist[1:]
				blockInIdx++
				continue
			}

			// build leaf
			var l btcacc.LeafData

			l.TxHash = btcacc.Hash(txin.PreviousOutPoint.Hash)
			l.Index = txin.PreviousOutPoint.Index

			l.Height = bnr.Rev.Txs[txinblock].TxIn[i].Height
			l.Coinbase = bnr.Rev.Txs[txinblock].TxIn[i].Coinbase
			// TODO get blockhash from headers here -- empty for now
			// l.BlockHash = getBlockHashByHeight(l.CbHeight >> 1)
			l.Amt = bnr.Rev.Txs[txinblock].TxIn[i].Amount
			l.PkScript = bnr.Rev.Txs[txinblock].TxIn[i].PKScript
			delLeaves = append(delLeaves, l)
			blockInIdx++
		}
	}
	return
}

// ParseBlockForDB gets a block and creates a ttlRawBlock to send to the DB worker
func ParseBlockForDB(
	bnr BlockAndRev) ttlRawBlock {

	var trb ttlRawBlock
	trb.blockHeight = bnr.Height

	var txoInBlock, txinInBlock uint32

	// if len(inskip) != 0 || len(outskip) != 0 {
	// fmt.Printf("h %d inskip %v outskip %v\n", bnr.Height, inskip, outskip)
	// }
	transactions := bnr.Blk.Transactions()
	inskip, outskip := bnr.Blk.DedupeBlock()
	// iterate through the transactions in a block
	for txInBlock, tx := range transactions {
		txid := tx.Hash()

		// for all the txouts, get their outpoint & index and throw that into
		// a db batch
		for txoInTx, txo := range tx.MsgTx().TxOut {
			if len(outskip) > 0 && txoInBlock == outskip[0] {
				// skip inputs in the txin skiplist
				// fmt.Printf("skipping output %s:%d\n", txid.String(), txoInTx)
				outskip = outskip[1:]
				txoInBlock++
				continue
			}
			if util.IsUnspendable(txo) {
				txoInBlock++
				continue
			}

			trb.newTxos = append(trb.newTxos,
				util.OutpointToBytes(wire.NewOutPoint(txid, uint32(txoInTx))))
			txoInBlock++
		}

		// for all the txins, throw that into the work as well; just a bunch of
		// outpoints
		for txinInTx, in := range tx.MsgTx().TxIn { // bit of a tounge twister
			if txInBlock == 0 {
				txinInBlock += uint32(len(tx.MsgTx().TxIn))
				break // skip coinbase input
			}
			if len(inskip) > 0 && txinInBlock == inskip[0] {
				// skip inputs in the txin skiplist
				// fmt.Printf("skipping input %s\n", in.PreviousOutPoint.String())
				inskip = inskip[1:]
				txinInBlock++
				continue
			}
			// append outpoint to slice
			trb.spentTxos = append(trb.spentTxos,
				util.OutpointToBytes(&in.PreviousOutPoint))
			// append start height to slice (get from rev data)
			trb.spentStartHeights = append(trb.spentStartHeights,
				bnr.Rev.Txs[txInBlock-1].TxIn[txinInTx].Height)

			txinInBlock++
		}
	}

	return trb
}
