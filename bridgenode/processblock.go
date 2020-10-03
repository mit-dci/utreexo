package bridgenode

import (
	"fmt"
	"sync"

	"github.com/btcsuite/btcd/wire"

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

// the data from a block about txo creation and deletion for TTL calculation
// this will be sent to the DB
type ttlRawBlock struct {
	blockHeight       int32
	newTxos           [][36]byte
	spentTxos         [][36]byte
	spentStartHeights []int32
}

// a TTLResult is the TTL data we learn once a txo is spent & it's lifetime
// can be written to its creation ublock
// When writing to the flat file, you want to seek to StartHeight, skip to
// the IndexWithinBlock'th entry, and write Duration (height- txBlockHeight)

// all the ttl result data from a block, after checking with the DB
// to be written to the flat file
type TTLResultBlock struct {
	Height  int32      // height of the block that consumed all the utxos
	Created []TxoStart // slice of
}

type TxoStart struct {
	TxBlockHeight    int32 // what block created the txo
	IndexWithinBlock int32 // index in that block where the txo is created
}

// blockToAddDel turns a block into add leaves and del leaves
func blockToAddDel(bnr BlockAndRev) (blockAdds []accumulator.Leaf,
	delLeaves []util.LeafData, err error) {

	inskip, outskip := util.DedupeBlock(&bnr.Blk)
	// fmt.Printf("inskip %v outskip %v\n", inskip, outskip)
	delLeaves, err = blockNRevToDelLeaves(bnr, inskip)
	if err != nil {
		return
	}

	// this is bridgenode, so don't need to deal with memorable leaves
	blockAdds = util.BlockToAddLeaves(bnr.Blk, nil, outskip, bnr.Height)

	return
}

// blockNRevToDelLeaves turns a block's inputs into delLeaves to be removed from the
// accumulator
func blockNRevToDelLeaves(bnr BlockAndRev, skiplist []uint32) (
	delLeaves []util.LeafData, err error) {

	// make sure same number of txs and rev txs (minus coinbase)
	if len(bnr.Blk.Transactions)-1 != len(bnr.Rev.Txs) {
		err = fmt.Errorf("genDels block %d %d txs but %d rev txs",
			bnr.Height, len(bnr.Blk.Transactions), len(bnr.Rev.Txs))
		return
	}

	var blockInIdx uint32
	for txinblock, tx := range bnr.Blk.Transactions {
		if txinblock == 0 {
			blockInIdx++ // coinbase tx always has 1 input
			continue
		}
		txinblock--
		// make sure there's the same number of txins
		if len(tx.TxIn) != len(bnr.Rev.Txs[txinblock].TxIn) {
			err = fmt.Errorf("genDels block %d tx %d has %d inputs but %d rev entries",
				bnr.Height, txinblock+1,
				len(tx.TxIn), len(bnr.Rev.Txs[txinblock].TxIn))
			return
		}
		// loop through inputs
		for i, txin := range tx.TxIn {
			// check if on skiplist.  If so, don't make leaf
			if len(skiplist) > 0 && skiplist[0] == blockInIdx {
				// fmt.Printf("skip %s\n", txin.PreviousOutPoint.String())
				skiplist = skiplist[1:]
				blockInIdx++
				continue
			}

			// build leaf
			var l util.LeafData

			l.Outpoint = txin.PreviousOutPoint
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

// genUData creates a block proof, calling forest.ProveBatch with the leaf indexes
// to get a batched inclusion proof from the accumulator. It then adds on the leaf data,
// to create a block proof which both proves inclusion and gives all utxo data
// needed for transaction verification.
func genUData(delLeaves []util.LeafData, f *accumulator.Forest, height int32) (
	ud util.UData, err error) {

	ud.UtxoData = delLeaves
	// make slice of hashes from leafdata
	delHashes := make([]accumulator.Hash, len(ud.UtxoData))
	for i, _ := range ud.UtxoData {
		delHashes[i] = ud.UtxoData[i].LeafHash()
		// fmt.Printf("del %s -> %x\n",
		// ud.UtxoData[i].Outpoint.String(), delHashes[i][:4])
	}
	// generate block proof. Errors if the tx cannot be proven
	// Should never error out with genproofs as it takes
	// blk*.dat files which have already been vetted by Bitcoin Core
	ud.AccProof, err = f.ProveBatch(delHashes)
	if err != nil {
		err = fmt.Errorf("genUData failed at block %d %s %s",
			height, f.Stats(), err.Error())
		return
	}

	if len(ud.AccProof.Targets) != len(delLeaves) {
		err = fmt.Errorf("genUData %d targets but %d leafData",
			len(ud.AccProof.Targets), len(delLeaves))
		return
	}

	// fmt.Printf(batchProof.ToString())
	// Optional Sanity check. Should never fail.

	// unsort := make([]uint64, len(ud.AccProof.Targets))
	// copy(unsort, ud.AccProof.Targets)
	// ud.AccProof.SortTargets()
	// ok := f.VerifyBatchProof(ud.AccProof)
	// if !ok {
	// 	return ud, fmt.Errorf("VerifyBatchProof failed at block %d", height)
	// }
	// ud.AccProof.Targets = unsort

	// also optional, no reason to do this other than bug checking

	// if !ud.Verify(f.ReconstructStats()) {
	// 	err = fmt.Errorf("height %d LeafData / Proof mismatch", height)
	// 	return
	// }
	return
}

// ParseBlockForDB gets a block and creates a ttlRawBlock to send to the DB worker
func ParseBlockForDB(bnr BlockAndRev, idxChan chan ttlRawBlock, wg *sync.WaitGroup) {
	var trb ttlRawBlock
	trb.blockHeight = bnr.Height

	// iterate through the transactions in a block
	for txInBlock, tx := range bnr.Blk.Transactions {
		txid := tx.TxHash()

		// for all the txouts, get their outpoint & index and throw that into
		// a db batch
		for txoInTx, _ := range tx.TxOut {
			trb.newTxos = append(trb.newTxos,
				util.OutpointToBytes(wire.NewOutPoint(&txid, uint32(txoInTx))))

			// for all the txins, throw that into the work as well; just a bunch of
			// outpoints

			for txinInTx, in := range tx.TxIn { // bit of a tounge twister
				if txInBlock == 0 {
					break // skip coinbase input
				}
				// append outpoint to slice
				trb.spentTxos = append(trb.spentTxos,
					util.OutpointToBytes(&in.PreviousOutPoint))
				// append start height to slice (get from rev data)
				trb.spentStartHeights = append(trb.spentStartHeights,
					bnr.Rev.Txs[txInBlock].TxIn[txinInTx].Height)
			}
		}
	}

	wg.Add(1)
	// send to dbworker to be written to ttldb asynchronously
	idxChan <- trb
}
