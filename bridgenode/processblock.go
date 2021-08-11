package bridgenode

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"sort"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/mit-dci/utreexo/btcacc"
	"github.com/mit-dci/utreexo/util"
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
	hashprefix [6]byte // txid prefix of txo being consumed
	idx        uint16  // outpoint index of txo being consumed
	height     int32   // creation height of txo being consumed
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
	startsAt uint16
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
	sort.Slice(s, func(a, b int) bool { return s[a].height < s[b].height })
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

// blockToAddDel turns a block into add leaves and del leaves
func blockToAddDel(bnr blockAndRev) (
	blockAdds []accumulator.Leaf, delLeaves []btcacc.LeafData, err error) {

	inCount, outCount, inskip, outskip := util.DedupeBlock(bnr.Blk)
	delLeaves, err = blockNRevToDelLeaves(bnr, inskip, inCount)
	if err != nil {
		return
	}

	// this is bridgenode, so don't need to deal with memorable leaves
	blockAdds = uwire.BlockToAddLeaves(bnr.Blk, nil, outskip, bnr.Height, outCount)

	return
}

// blockNRevToDelLeaves turns a block's inputs into delLeaves to be removed from the
// accumulator
func blockNRevToDelLeaves(bnr blockAndRev, skiplist []uint32, inCount int) (
	delLeaves []btcacc.LeafData, err error) {

	delLeaves = make([]btcacc.LeafData, 0, inCount-len(skiplist))

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
