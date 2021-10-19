package bridgenode

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"sort"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/mit-dci/utreexo/btcacc"
	uwire "github.com/mit-dci/utreexo/wire"

	"github.com/mit-dci/utreexo/accumulator"
)

/*
New setup:  all flat files, no db.

block & rev comes in.  From it, get a list of txs with # outputs each,
and a list of spent outpoints.

For the output side, we take every txid and associated # of outputs.
Sort the txids, then write the sorted txid/numout tuples to a flat file.
Make a little index file to keep track of this flat file.

For the input side, we know the TTL for every outpoint, we just need to know
where to write it.  Seek into the txid flatfile to the creating block.
Do an interpolation search for the txid, then you'll know where that txid
starts in the block, and add that to the index of your outpoint to get the
total in-block output index.  Seek to the TTL file, and write it there.

There is a search, but it's interpolation over TXIDs.  I know there's something
like 6K outputs per block and 2500 txs per block, so the search only needs to
go through 2500 elements.  Binary search, log(n) would be around 11 searches,
interpolation search is log(log(n)) so 3 to 4.

Is the 3-4 nearby seeks per lookup (maybe 15-20K seeks per block) faster than
levelDB?  We'll find out.

It sounds bad for spinning disks, and it might be, but these seeks are very
close to each other.  The sorted txid block is like 40K, and we could bring that
down further if needed.

*/

// ttlWriteBlock is data about the creation of txids (& their position) in a block
type ttlWriteBlock struct {
	createHeight int32    // height of this block, creating txos
	mTxids       []miniTx // one for tx
}

// ttlLookupBlock is the data from a block about txo creation and deletion
// needed for TTL calculations
type ttlLookupBlock struct {
	destroyHeight int32    // height of this block, destroying the txos
	spentTxos     []miniIn // one for every input
}

// miniIn are miniature outpoints, for the spent side of the block.
// 6 bytes of txid prefix, then 2 bytes of position.
type miniIn struct {
	hashprefix   [6]byte // txid prefix of txo being consumed
	idx          uint16  // outpoint index of txo being consumed
	createHeight int32   // creation height of txo being consumed
}

// to int... which will turn into a float
func (mt *miniIn) hashToUint64() uint64 {
	// welll this is ugly but probably the fastest way
	return uint64(mt.hashprefix[5]) | uint64(mt.hashprefix[4])<<8 |
		uint64(mt.hashprefix[3])<<16 | uint64(mt.hashprefix[2])<<24 |
		uint64(mt.hashprefix[1])<<32 | uint64(mt.hashprefix[0])<<40
}

func miniBytesToUint64(b [6]byte) uint64 {
	return uint64(b[5]) | uint64(b[4])<<8 | uint64(b[3])<<16 |
		uint64(b[2])<<24 | uint64(b[1])<<32 | uint64(b[0])<<40
}

type miniTx struct {
	txid     *chainhash.Hash
	startsAt uint16 // the 0th output in this tx is the _th input in the block
	// note that there COULD BE more than 65K outputs in a block and
	// this should probably deal with that.  They'd be "silly" outputs though
	// TODO move to 32 bits, or really 17 would be plenty
}

// miniTx serialization is 6 bytes of the txid, then 2 bytes for a uint16
// TODO there are probably no 6 byte prefix collisions in any block.
// But there could be someday, so deal with that...
func (wb *ttlWriteBlock) serialize(w io.Writer) error {

	s := fmt.Sprintf("ttl wb h %d %d txs\n", wb.createHeight, len(wb.mTxids))
	for _, mt := range wb.mTxids {
		_, err := w.Write(mt.txid[:6])
		if err != nil {
			return err
		}

		err = binary.Write(w, binary.BigEndian, mt.startsAt)
		if err != nil {
			return err
		}
		s += fmt.Sprintf("tx %x starts at idxinblock %d\n", mt.txid[:6], mt.startsAt)
	}
	// fmt.Printf(s)
	return nil
}

// miniTx serialization is 6 bytes of the txid, then 2 bytes for a uint16
// TODO there are probably no 6 byte prefix collisions in any block.
// But there could be someday, so deal with that...
func (mt *miniTx) serialize(w io.Writer) error {
	_, err := w.Write(mt.txid[:6])
	if err != nil {
		return err
	}

	err = binary.Write(w, binary.BigEndian, mt.startsAt)
	if err != nil {
		return err
	}
	return nil
}

func sortTxids(s []miniTx) {
	sort.Slice(s, func(a, b int) bool {
		return bytes.Compare(s[a].txid[:], s[b].txid[:]) < 0
	})
}

func sortMiniIns(s []miniIn) {
	sort.Slice(s, func(a, b int) bool { return s[a].createHeight < s[b].createHeight })
}

// a TTLResult is the TTL data we learn once a txo is spent & it's lifetime
// can be written to its creation ublock
// When writing to the flat file, you want to seek to StartHeight, skip to
// the IndexWithinBlock'th entry, and write Duration (height- txBlockHeight)

// all the ttl result data from a block, after checking with the DB
// to be written to the flat file
type ttlResultBlock struct {
	destroyHeight int32       // height of the block that consumed all the utxos
	results       []ttlResult // slice of txo creation info
}

type ttlResult struct {
	createHeight     int32  // what block created the txo
	indexWithinBlock uint16 // index in that block where the txo is created
}

func (bnr *blockAndRev) toAddDel() (
	blockAdds []accumulator.Leaf, delLeaves []btcacc.LeafData, err error) {

	delLeaves, err = bnr.toDelLeaves()
	if err != nil {
		return
	}

	// this is bridgenode, so don't need to deal with memorable leaves
	blockAdds = uwire.BlockToAddLeaves(
		bnr.Blk, nil, bnr.outSkipList, bnr.Height, bnr.outCount)

	// if bnr.Height == 106 {
	// fmt.Printf("h %d outskip %v\n", bnr.Height, bnr.outSkipList)
	// }
	return

}

// blockNRevToDelLeaves turns a block's inputs into delLeaves to be removed from the
// accumulator
func (bnr *blockAndRev) toDelLeaves() (
	delLeaves []btcacc.LeafData, err error) {

	// finish early if there's nothing to prove
	if bnr.inCount-uint32(len(bnr.inSkipList)) == 0 {
		return
	}

	delLeaves = make([]btcacc.LeafData, 0, bnr.inCount-uint32(len(bnr.inSkipList)))
	inskip := bnr.inSkipList
	// we never modify the contents of this slice
	// only the borders of the slice, so this shouldn't change bnr.inSkipList

	// make sure same number of txs and rev txs (minus coinbase)
	if len(bnr.Blk.Transactions())-1 != len(bnr.Rev.Txs) {
		err = fmt.Errorf("genDels block %d %d txs but %d rev txs",
			bnr.Height, len(bnr.Blk.Transactions()), len(bnr.Rev.Txs))
		return
	}

	var inputInBlock uint32
	for txInBlock, tx := range bnr.Blk.Transactions() {
		if txInBlock == 0 {
			// make sure input 0 in the block is skipped
			if len(inskip) < 1 || inskip[0] != 0 {
				err = fmt.Errorf("input 0 (coinbase) wasn't skipped")
				return
			}
			inskip = inskip[1:]
			inputInBlock++
			continue
		}

		txInBlock--
		// make sure there's the same number of txins
		if len(tx.MsgTx().TxIn) != len(bnr.Rev.Txs[txInBlock].TxIn) {
			err = fmt.Errorf("genDels h %d tx %d: %d inputs but %d rev entries",
				bnr.Height, txInBlock,
				len(tx.MsgTx().TxIn), len(bnr.Rev.Txs[txInBlock].TxIn))
			return
		}

		// loop through inputs
		for i, txin := range tx.MsgTx().TxIn {
			// check if on inskip.  If so, don't make leaf
			if len(inskip) > 0 && inskip[0] == inputInBlock {
				// fmt.Printf("skip %s\n", txin.PreviousOutPoint.String())
				inskip = inskip[1:]
				inputInBlock++
				continue
			}

			// build leaf
			var l btcacc.LeafData

			l.TxHash = btcacc.Hash(txin.PreviousOutPoint.Hash)
			l.Index = txin.PreviousOutPoint.Index

			l.Height = bnr.Rev.Txs[txInBlock].TxIn[i].Height
			l.Coinbase = bnr.Rev.Txs[txInBlock].TxIn[i].Coinbase
			// TODO get blockhash from headers here -- empty for now
			// l.BlockHash = getBlockHashByHeight(l.CbHeight >> 1)
			l.Amt = bnr.Rev.Txs[txInBlock].TxIn[i].Amount
			l.PkScript = bnr.Rev.Txs[txInBlock].TxIn[i].PKScript
			delLeaves = append(delLeaves, l)
			inputInBlock++
		}
	}
	return
}

func (bnr *blockAndRev) toString() string {
	s := fmt.Sprintf("h %d %d out skip %v %d in skip %v\n",
		bnr.Height, bnr.outCount, bnr.outSkipList,
		bnr.inCount, bnr.inSkipList)
	outskipped := 0
	txoInBlock := 0
	txinInBlock := 0
	inSkipPos, outSkipPos := 0, 0

	shouldoutskip := len(bnr.outSkipList)
	block := bnr.Blk.MsgBlock()
	for txnum, tx := range block.Transactions {
		txid := tx.TxHash()
		s += fmt.Sprintf("tx %d ------------------\n", txnum)

		maxRow := len(tx.TxIn)
		if len(tx.TxOut) > maxRow {
			maxRow = len(tx.TxOut)
		}
		for rowInTx := 0; rowInTx < maxRow; rowInTx++ {
			// s += fmt.Sprintf("txinnum %d txonum %d inskip %v outskip %v\n",
			// txinInBlock, txoInBlock, bnr.inSkipList, bnr.outSkipList)
			if rowInTx < len(tx.TxIn) {
				if len(bnr.inSkipList) > inSkipPos &&
					uint32(txinInBlock) == bnr.inSkipList[inSkipPos] {
					s += fmt.Sprintf("SKIP ")
					inSkipPos++
				} else {
					s += fmt.Sprintf("     ")
				}
				s += fmt.Sprintf("in %x:%d\t",
					tx.TxIn[rowInTx].PreviousOutPoint.Hash[:6],
					tx.TxIn[rowInTx].PreviousOutPoint.Index&0xffff)
				txinInBlock++
			} else {
				s += fmt.Sprintf("\t\t\t")
			}
			if rowInTx < len(tx.TxOut) {
				if len(bnr.outSkipList) > outSkipPos &&
					uint32(txoInBlock) == bnr.outSkipList[outSkipPos] {
					s += fmt.Sprintf("SKIP ")
					outskipped++
					outSkipPos++
				} else {
					s += fmt.Sprintf("     ")
				}
				s += fmt.Sprintf("out %x:%d\n", txid[:6], rowInTx)
				txoInBlock++
			} else {
				s += fmt.Sprintf("\n")
			}
		}
	}
	if outskipped != shouldoutskip {
		s += fmt.Sprintf("h %d skipped %d but supposed to skip %d\n",
			bnr.Height, outskipped, shouldoutskip)
		fmt.Printf(s)
		panic("bad skip")
	}
	return s
}
