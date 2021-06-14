// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chain

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"sort"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/mit-dci/utreexo/accumulator"
	"github.com/mit-dci/utreexo/btcacc"
	"github.com/mit-dci/utreexo/wire"
)

// OutOfRangeError describes an error due to accessing an element that is out
// of range.
type OutOfRangeError string

// BlockHeightUnknown is the value returned for a block height that is unknown.
// This is typically because the block has not been inserted into the main chain
// yet.
const BlockHeightUnknown = int32(-1)

// Error satisfies the error interface and prints human-readable errors.
func (e OutOfRangeError) Error() string {
	return string(e)
}

// Block defines a bitcoin block that provides easier and more efficient
// manipulation of raw blocks.  It also memoizes hashes for the block and its
// transactions on their first access so subsequent accesses don't have to
// repeat the relatively expensive hashing operations.
type Block struct {
	msgBlock                 *wire.MsgBlock  // Underlying MsgBlock
	serializedBlock          []byte          // Serialized bytes for the block
	serializedBlockNoWitness []byte          // Serialized bytes for block w/o witness data
	blockHash                *chainhash.Hash // Cached block hash
	blockHeight              int32           // Height in the main block chain
	transactions             []*Tx           // Transactions
	inskip                   []uint32        // all sstxoIndexes of the TxIns that are unspendable or a same block spend
	outskip                  []uint32        // all sstxoIndexes of the TxOuts that are unspendable or a same block spend
	skiplistGenerated        bool            // ALL wrapped transactions generated
	txnsGenerated            bool            // ALL wrapped transactions generated
}

// MsgBlock returns the underlying wire.MsgBlock for the Block.
func (b *Block) MsgBlock() *wire.MsgBlock {
	// Return the cached block.
	return b.msgBlock
}

// Bytes returns the serialized bytes for the Block.  This is equivalent to
// calling Serialize on the underlying wire.MsgBlock, however it caches the
// result so subsequent calls are more efficient.
func (b *Block) Bytes() ([]byte, error) {
	// Return the cached serialized bytes if it has already been generated.
	if len(b.serializedBlock) != 0 {
		return b.serializedBlock, nil
	}

	// Serialize the MsgBlock.
	w := bytes.NewBuffer(make([]byte, 0, b.msgBlock.SerializeSize()))
	err := b.msgBlock.Serialize(w)
	if err != nil {
		return nil, err
	}
	serializedBlock := w.Bytes()

	// Cache the serialized bytes and return them.
	b.serializedBlock = serializedBlock
	return serializedBlock, nil
}

// BytesNoWitness returns the serialized bytes for the block with transactions
// encoded without any witness data.
func (b *Block) BytesNoWitness() ([]byte, error) {
	// Return the cached serialized bytes if it has already been generated.
	if len(b.serializedBlockNoWitness) != 0 {
		return b.serializedBlockNoWitness, nil
	}

	// Serialize the MsgBlock.
	var w bytes.Buffer
	err := b.msgBlock.SerializeNoWitness(&w)
	if err != nil {
		return nil, err
	}
	serializedBlock := w.Bytes()

	// Cache the serialized bytes and return them.
	b.serializedBlockNoWitness = serializedBlock
	return serializedBlock, nil
}

// Hash returns the block identifier hash for the Block.  This is equivalent to
// calling BlockHash on the underlying wire.MsgBlock, however it caches the
// result so subsequent calls are more efficient.
func (b *Block) Hash() *chainhash.Hash {
	// Return the cached block hash if it has already been generated.
	if b.blockHash != nil {
		return b.blockHash
	}

	// Cache the block hash and return it.
	hash := b.msgBlock.BlockHash()
	b.blockHash = &hash
	return &hash
}

// Tx returns a wrapped transaction (btcutil.Tx) for the transaction at the
// specified index in the Block.  The supplied index is 0 based.  That is to
// say, the first transaction in the block is txNum 0.  This is nearly
// equivalent to accessing the raw transaction (wire.MsgTx) from the
// underlying wire.MsgBlock, however the wrapped transaction has some helpful
// properties such as caching the hash so subsequent calls are more efficient.
func (b *Block) Tx(txNum int) (*Tx, error) {
	// Ensure the requested transaction is in range.
	numTx := uint64(len(b.msgBlock.Transactions))
	if txNum < 0 || uint64(txNum) >= numTx {
		str := fmt.Sprintf("transaction index %d is out of range - max %d",
			txNum, numTx-1)
		return nil, OutOfRangeError(str)
	}

	// Generate slice to hold all of the wrapped transactions if needed.
	if len(b.transactions) == 0 {
		b.transactions = make([]*Tx, numTx)
	}

	// Return the wrapped transaction if it has already been generated.
	if b.transactions[txNum] != nil {
		return b.transactions[txNum], nil
	}

	// Generate and cache the wrapped transaction and return it.
	newTx := NewTx(b.msgBlock.Transactions[txNum])
	newTx.SetIndex(txNum)
	b.transactions[txNum] = newTx
	return newTx, nil
}

// Transactions returns a slice of wrapped transactions (btcutil.Tx) for all
// transactions in the Block.  This is nearly equivalent to accessing the raw
// transactions (wire.MsgTx) in the underlying wire.MsgBlock, however it
// instead provides easy access to wrapped versions (btcutil.Tx) of them.
func (b *Block) Transactions() []*Tx {
	// Return transactions if they have ALL already been generated.  This
	// flag is necessary because the wrapped transactions are lazily
	// generated in a sparse fashion.
	if b.txnsGenerated {
		return b.transactions
	}

	// Generate slice to hold all of the wrapped transactions if needed.
	if len(b.transactions) == 0 {
		b.transactions = make([]*Tx, len(b.msgBlock.Transactions))
	}

	var sstxoIndex, skipIndex uint32 //, indexToPut uint32
	hashes := make([]chainhash.Hash, 0, len(b.msgBlock.Transactions))
	b.inskip, b.outskip = dedupeBlock(b, &hashes)

	b.skiplistGenerated = true

	skipIdx := 0
	skiplistLen := len(b.outskip)
	// Generate and cache the wrapped transactions for all that haven't
	// already been done.
	for i, tx := range b.transactions {
		if tx == nil {
			newTx := NewTx(b.msgBlock.Transactions[i])
			newTx.SetIndex(i)
			newTx.txHash = &hashes[i]

			for outIdx, txo := range b.msgBlock.Transactions[i].TxOut {
				var indexToPut int16

				// Check if the output is unspendable or is a
				// same block spend
				if skiplistLen != skipIdx && skipIndex == b.outskip[skipIdx] {
					//outskip = outskip[1:]
					skipIdx++
					indexToPut = SSTxoIndexNA
				}
				if isUnspendable(txo) {
					indexToPut = SSTxoIndexNA
				}
				if indexToPut != SSTxoIndexNA {
					indexToPut = int16(sstxoIndex)
					sstxoIndex++
				}

				newTx.txos[outIdx].sstxoIndex = indexToPut

				skipIndex++
			}
			b.transactions[i] = newTx
		}
	}

	b.txnsGenerated = true
	return b.transactions
}

// IsUnspendable determines whether a tx is spendable or not.
// returns true if spendable, false if unspendable.
// NOTE: Since the btcd isUnspendable and bitcoind isUnspendable
// differs in they'll mark unspendable we have a separate function
// that follows the same behavior as bitcoind's function
func isUnspendable(o *wire.TxOut) bool {
	switch {
	case len(o.PkScript) > 10000: // len 0 is OK, spendable
		return true
	case len(o.PkScript) > 0 && o.PkScript[0] == 0x6a: // OP_RETURN is 0x6a
		return true
	default:
		return false
	}
}

// DedupeBlock returns the inskip and outskip for this block
func (b *Block) DedupeBlock() ([]uint32, []uint32) {
	if b.skiplistGenerated {
		return b.inskip, b.outskip
	}

	b.inskip, b.outskip = dedupeBlock(b, nil)
	return b.inskip, b.outskip
}

// DedupeBlock takes a bitcoin block, and returns two int slices: the indexes of
// inputs, and idexes of outputs which can be removed.  These are indexes
// within the block as a whole, even the coinbase tx.
// So the coinbase tx in & output numbers affect the skip lists even though
// the coinbase ins/outs can never be deduped.  it's simpler that way.
func dedupeBlock(blk *Block, hashes *[]chainhash.Hash) (inskip []uint32, outskip []uint32) {
	var i uint32
	// wire.Outpoints are comparable with == which is nice.
	inmap := make(map[wire.OutPoint]uint32)

	// go through txs then inputs building map
	for cbif0, tx := range blk.msgBlock.Transactions {
		if cbif0 == 0 { // coinbase tx can't be deduped
			i++ // coinbase has 1 input
			continue
		}
		for _, in := range tx.TxIn {
			inmap[in.PreviousOutPoint] = i
			i++
		}
	}

	i = 0
	// start over, go through outputs finding skips
	for cbif0, tx := range blk.msgBlock.Transactions {
		hash := tx.TxHash()
		if hashes != nil {
			*hashes = append(*hashes, hash)
		}
		if cbif0 == 0 { // coinbase tx can't be deduped
			i += uint32(len(tx.TxOut)) // coinbase can have multiple inputs
			continue
		}

		for outidx, _ := range tx.TxOut {
			op := wire.OutPoint{Hash: hash, Index: uint32(outidx)}
			inpos, exists := inmap[op]
			if exists {
				inskip = append(inskip, inpos)
				outskip = append(outskip, i)
			}
			i++
		}
	}

	// sort inskip list, as it's built in order consumed not created
	sortUint32s(inskip)
	return
}

// it'd be cool if you just had .sort() methods on slices of builtin types...
func sortUint32s(s []uint32) {
	sort.Slice(s, func(a, b int) bool { return s[a] < s[b] })
}

// TxHash returns the hash for the requested transaction number in the Block.
// The supplied index is 0 based.  That is to say, the first transaction in the
// block is txNum 0.  This is equivalent to calling TxHash on the underlying
// wire.MsgTx, however it caches the result so subsequent calls are more
// efficient.
func (b *Block) TxHash(txNum int) (*chainhash.Hash, error) {
	// Attempt to get a wrapped transaction for the specified index.  It
	// will be created lazily if needed or simply return the cached version
	// if it has already been generated.
	tx, err := b.Tx(txNum)
	if err != nil {
		return nil, err
	}

	// Defer to the wrapped transaction which will return the cached hash if
	// it has already been generated.
	return tx.Hash(), nil
}

// TxLoc returns the offsets and lengths of each transaction in a raw block.
// It is used to allow fast indexing into transactions within the raw byte
// stream.
func (b *Block) TxLoc() ([]wire.TxLoc, error) {
	rawMsg, err := b.Bytes()
	if err != nil {
		return nil, err
	}
	rbuf := bytes.NewBuffer(rawMsg)

	var mblock wire.MsgBlock
	txLocs, err := mblock.DeserializeTxLoc(rbuf)
	if err != nil {
		return nil, err
	}
	return txLocs, err
}

// Height returns the saved height of the block in the block chain.  This value
// will be BlockHeightUnknown if it hasn't already explicitly been set.
func (b *Block) Height() int32 {
	return b.blockHeight
}

// SetHeight sets the height of the block in the block chain.
func (b *Block) SetHeight(height int32) {
	b.blockHeight = height
}

// BlockToAdds turns all the new utxos in a msgblock into leafTxos
// uses remember slice up to number of txos, but doesn't check that it's the
// right length.  Similar with skiplist, doesn't check it.
func BlockToAddLeaves(blk *Block,
	remember []bool, skiplist []uint32,
	height int32) (leaves []accumulator.Leaf) {

	var txonum uint32
	for coinbaseif0, tx := range blk.Transactions() {
		// cache txid aka txhash
		txid := tx.Hash()
		for i, out := range tx.MsgTx().TxOut {
			// Skip all the OP_RETURNs
			if IsUnspendable(out) {
				txonum++
				continue
			}
			// Skip txos on the skip list
			if len(skiplist) > 0 && skiplist[0] == txonum {
				skiplist = skiplist[1:]
				txonum++
				continue
			}

			var l btcacc.LeafData
			// TODO put blockhash back in -- leaving empty for now!
			// l.BlockHash = bh
			l.TxHash = btcacc.Hash(*txid)
			l.Index = uint32(i)
			l.Height = height
			if coinbaseif0 == 0 {
				l.Coinbase = true
			}
			l.Amt = out.Value
			l.PkScript = out.PkScript
			uleaf := accumulator.Leaf{Hash: l.LeafHash()}
			if uint32(len(remember)) > txonum {
				uleaf.Remember = remember[txonum]
			}
			leaves = append(leaves, uleaf)
			txonum++
		}
	}
	return
}

// IsUnspendable determines whether a tx is spendable or not.
// returns true if spendable, false if unspendable.
func IsUnspendable(o *wire.TxOut) bool {
	switch {
	case len(o.PkScript) > 10000: //len 0 is OK, spendable
		return true
	case len(o.PkScript) > 0 && o.PkScript[0] == 0x6a: // OP_RETURN is 0x6a
		return true
	default:
		return false
	}
}

// turns an outpoint into a 36 byte... mixed endian thing.
// (the 32 bytes txid is "reversed" and the 4 byte index is in order (big)
func OutpointToBytes(op *wire.OutPoint) (b [36]byte) {
	copy(b[0:32], op.Hash[:])
	binary.BigEndian.PutUint32(b[32:36], op.Index)
	return
}

// NewBlock returns a new instance of a bitcoin block given an underlying
// wire.MsgBlock.  See Block.
func NewBlock(msgBlock *wire.MsgBlock) *Block {
	return &Block{
		msgBlock:    msgBlock,
		blockHeight: BlockHeightUnknown,
	}
}

// NewBlockFromBytes returns a new instance of a bitcoin block given the
// serialized bytes.  See Block.
func NewBlockFromBytes(serializedBlock []byte) (*Block, error) {
	br := bytes.NewReader(serializedBlock)
	b, err := NewBlockFromReader(br)
	if err != nil {
		return nil, err
	}
	b.serializedBlock = serializedBlock
	return b, nil
}

// NewBlockFromReader returns a new instance of a bitcoin block given a
// Reader to deserialize the block.  See Block.
func NewBlockFromReader(r io.Reader) (*Block, error) {
	// Deserialize the bytes into a MsgBlock.
	var msgBlock wire.MsgBlock
	err := msgBlock.Deserialize(r)
	if err != nil {
		return nil, err
	}

	b := Block{
		msgBlock:    &msgBlock,
		blockHeight: BlockHeightUnknown,
	}
	return &b, nil
}

// NewBlockFromBlockAndBytes returns a new instance of a bitcoin block given
// an underlying wire.MsgBlock and the serialized bytes for it.  See Block.
func NewBlockFromBlockAndBytes(msgBlock *wire.MsgBlock, serializedBlock []byte) *Block {
	return &Block{
		msgBlock:        msgBlock,
		serializedBlock: serializedBlock,
		blockHeight:     BlockHeightUnknown,
	}
}
