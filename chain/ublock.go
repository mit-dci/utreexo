package chain

import (
	"bytes"
	"fmt"
	"io"
	"sync"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	btcdwire "github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"github.com/mit-dci/utreexo/btcacc"
	"github.com/mit-dci/utreexo/wire"
)

// UBlock represents a utreexo block. It mimicks the behavior of Block in block.go
type UBlock struct {
	uData *btcacc.UData
	block *Block

	serializedUBlock          []byte         // Serialized bytes for the block
	serializedUBlockNoWitness []byte         // Serialized bytes for block w/o witness data
	blockHash                 chainhash.Hash // Cached block hash
	blockHeight               int32          // Height in the main block chain
	transactions              []*Tx          // Transactions
	txnsGenerated             bool           // ALL wrapped transactions generated
	blockGenerated            bool
}

// UData returns the underlying udata
func (ub *UBlock) UData() *btcacc.UData {
	return ub.uData
}

func (ub *UBlock) Bytes() ([]byte, error) {
	// Return the cached serialized bytes if it has already been generated.
	if len(ub.serializedUBlock) != 0 {
		return ub.serializedUBlock, nil
	}

	// Serialize the MsgBlock.
	w := bytes.NewBuffer(make([]byte, 0,
		ub.block.MsgBlock().SerializeSize()+ub.uData.SerializeSizeVarInt()))
	err := ub.block.MsgBlock().Serialize(w)
	if err != nil {
		return nil, err
	}
	err = ub.uData.Encode(w)
	if err != nil {
		return nil, err
	}
	serializedUBlock := w.Bytes()

	// Cache the serialized bytes and return them.
	ub.serializedUBlock = serializedUBlock
	return serializedUBlock, nil
}

// BytesNoWitness returns the serialized bytes for the block with transactions
// encoded without any witness data.
func (ub *UBlock) BytesNoWitness() ([]byte, error) {
	// Return the cached serialized bytes if it has already been generated.
	if len(ub.serializedUBlockNoWitness) != 0 {
		return ub.serializedUBlockNoWitness, nil
	}

	// Serialize the MsgBlock.
	var w bytes.Buffer
	err := ub.block.MsgBlock().SerializeNoWitness(&w)
	if err != nil {
		return nil, err
	}
	err = ub.uData.Encode(&w)
	if err != nil {
		return nil, err
	}
	serializedUBlock := w.Bytes()

	// Cache the serialized bytes and return them.
	ub.serializedUBlockNoWitness = serializedUBlock
	return serializedUBlock, nil
}

// Hash returns the block identifier hash for the Block.  This is equivalent to
// calling BlockHash on the underlying wire.MsgBlock, however it caches the
// result so subsequent calls are more efficient.
func (ub *UBlock) Hash() *chainhash.Hash {
	// Return the cached block hash if it has already been generated.
	if ub.block.blockHash != nil {
		return ub.block.blockHash
	}

	// Cache the block hash and return it.
	hash := ub.block.MsgBlock().BlockHash()
	ub.block.blockHash = &hash
	return &hash
}

// Height returns the saved height of the block in the block chain.  This value
// will be BlockHeightUnknown if it hasn't already explicitly been set.
func (b *UBlock) Height() int32 {
	return b.block.blockHeight
}

// SetHeight sets the height of the block in the block chain.
func (b *UBlock) SetHeight(height int32) {
	b.block.blockHeight = height
}

// NewUBlock returns a new instance of a bitcoin block given an underlying
// wire.MsgUBlock.  See UBlock.
func NewUBlock(msgUBlock *wire.MsgUBlock) *UBlock {
	return &UBlock{
		block:       NewBlock(&msgUBlock.MsgBlock),
		uData:       &msgUBlock.UtreexoData,
		blockHeight: BlockHeightUnknown,
	}
}

// NewUBlockFromReader returns a new instance of a utreexo block given a
// Reader to deserialize the ublock.  See UBlock.
func NewUBlockFromReader(r io.Reader) (*UBlock, error) {
	// Deserialize the bytes into a MsgBlock.
	var msgUBlock wire.MsgUBlock
	err := msgUBlock.Deserialize(r)
	if err != nil {
		return nil, err
	}

	ub := UBlock{
		block:       NewBlock(&msgUBlock.MsgBlock),
		uData:       &msgUBlock.UtreexoData,
		blockHeight: BlockHeightUnknown,
	}
	return &ub, nil
}

// NewUBlockFromBlockAndBytes returns a new instance of a utreexo block given
// an underlying wire.MsgUBlock and the serialized bytes for it.  See UBlock.
func NewUBlockFromBlockAndBytes(msgUBlock *wire.MsgUBlock, serializedUBlock []byte) *UBlock {
	return &UBlock{
		block:            NewBlock(&msgUBlock.MsgBlock),
		uData:            &msgUBlock.UtreexoData,
		serializedUBlock: serializedUBlock,
		blockHeight:      BlockHeightUnknown,
	}
}

// Block builds a block from the UBlock. For compatibility with some functions
// that want a block
func (ub *UBlock) Block() *Block {
	return ub.block
}

// ProofSanity checks the consistency of a UBlock
func (ub *UBlock) ProofSanity(inputSkipList []uint32, nl uint64, h uint8) error {
	// get the outpoints that need proof
	proveOPs := BlockToDelOPs(ub.Block().MsgBlock(), inputSkipList)

	// ensure that all outpoints are provided in the extradata
	if len(proveOPs) != len(ub.UData().Stxos) {
		err := fmt.Errorf("height %d %d outpoints need proofs but only %d proven\n",
			ub.UData().Height, len(proveOPs), len(ub.UData().Stxos))
		return err
	}
	for i, _ := range ub.UData().Stxos {
		if chainhash.Hash(proveOPs[i].Hash) != chainhash.Hash(ub.UData().Stxos[i].TxHash) ||
			proveOPs[i].Index != ub.UData().Stxos[i].Index {
			err := fmt.Errorf("block/utxoData mismatch %s v %s\n",
				proveOPs[i].String(), ub.UData().Stxos[i].OPString())
			return err
		}
	}
	// derive leafHashes from leafData
	if !ub.UData().ProofSanity(nl, h) {
		return fmt.Errorf("height %d LeafData / Proof mismatch", ub.UData().Height)
	}

	return nil
}

// BlockToDelOPs gives all the UTXOs in a block that need proofs in order to be
// deleted.  All txinputs except for the coinbase input and utxos created
// within the same block (on the skiplist)
func BlockToDelOPs(
	blk *wire.MsgBlock, skiplist []uint32) (delOPs []wire.OutPoint) {

	var blockInIdx uint32
	for txinblock, tx := range blk.Transactions {
		if txinblock == 0 {
			blockInIdx++ // coinbase tx always has 1 input
			continue
		}

		// loop through inputs
		for _, txin := range tx.TxIn {
			// check if on skiplist.  If so, don't make leaf
			if len(skiplist) > 0 && skiplist[0] == blockInIdx {
				skiplist = skiplist[1:]
				blockInIdx++
				continue
			}

			delOPs = append(delOPs, txin.PreviousOutPoint)
			blockInIdx++
		}
	}
	return
}

// toBtcdTx is for keeping compatibility with the btcdTx as we transition to
// internalizing all the necessary code from the utcd repo to the utreexo repo.
// toBtcdTx serializes and then deserializes to change the Tx to the btcutil.Tx.
// TODO get rid of this as this function. Inefficient and shouldn't be needed down the road.
func toBtcdTx(tx Tx) (*btcutil.Tx, error) {
	var buf bytes.Buffer
	err := tx.MsgTx().Serialize(&buf)
	if err != nil {
		return nil, err
	}
	btcdTx, err := btcutil.NewTxFromBytes(buf.Bytes())
	if err != nil {
		return nil, err
	}
	btcdTx.SetIndex(tx.Index())

	return btcdTx, nil
}

// CheckBlock does all internal block checks for a UBlock
// right now checks the signatures
func (ub *UBlock) CheckBlock(outskip []uint32, p *chaincfg.Params) bool {
	// NOTE Whatever happens here is done a million times
	// be efficient here
	view := ub.ToUtxoView()
	viewMap := view.Entries()
	var txonum uint32

	sigCache := txscript.NewSigCache(0)
	hashCache := txscript.NewHashCache(0)

	for txnum, tx := range ub.Block().Transactions() {
		outputsInTx := uint32(len(tx.MsgTx().TxOut))
		if txnum == 0 {
			txonum += outputsInTx
			continue // skip checks for coinbase TX for now.  Or maybe it'll work?
		}
		/* add txos to the UtxoView if they're also consumed in this block
		(will be on the output skiplist from DedupeBlock)
		The order we do this in should ensure that a incorrectly ordered
		sequence (tx 5 spending tx 8) will fail here.
		*/
		for len(outskip) > 0 && outskip[0] < txonum+outputsInTx {
			idx := outskip[0] - txonum
			skipTxo := btcdwire.NewTxOut(tx.MsgTx().TxOut[idx].Value,
				tx.MsgTx().TxOut[idx].PkScript)
			skippedEntry := blockchain.NewUtxoEntry(
				skipTxo, ub.UData().Height, false)
			skippedOutpoint := btcdwire.OutPoint{Hash: *tx.Hash(), Index: idx}
			viewMap[skippedOutpoint] = skippedEntry
			outskip = outskip[1:] // pop off from output skiplist
		}
		txonum += outputsInTx
	}

	var wg sync.WaitGroup
	wg.Add(len(ub.Block().Transactions()) - 1) // subtract coinbase
	for txnum, tx := range ub.Block().Transactions() {
		if txnum == 0 {
			continue // skip checks for coinbase TX for now.  Or maybe it'll work?
		}
		btcdTx, err := toBtcdTx(*tx)
		if err != nil {
			// just panic if we aren't able to get a btcutil.Tx.
			// TODO This whole translation is going to be gotten rid of later down the road.
			panic(err)
		}
		go func(w *sync.WaitGroup, tx *btcutil.Tx) {
			// hardcoded testnet3 for now
			_, err := blockchain.CheckTransactionInputs(
				tx, ub.UData().Height, view, p)
			if err != nil {
				fmt.Printf("Tx %s fails CheckTransactionInputs: %s\n",
					tx.Hash().String(), err.Error())
				panic(err)
			}

			// no scriptflags for now
			err = blockchain.ValidateTransactionScripts(
				tx, view, 0, sigCache, hashCache)
			if err != nil {
				fmt.Printf("Tx %s fails ValidateTransactionScripts: %s\n",
					tx.Hash().String(), err.Error())
				panic(err)
			}
			w.Done()
		}(&wg, btcdTx)
	}
	wg.Wait()

	return true
}

// ToUtxoView converts a UData into a btcd blockchain.UtxoViewpoint
// all the data is there, just a bit different format.
// Note that this needs blockchain.NewUtxoEntry() in btcd
func (ub *UBlock) ToUtxoView() *blockchain.UtxoViewpoint {
	v := blockchain.NewUtxoViewpoint()
	m := v.Entries()
	// loop through leafDatas and convert them into UtxoEntries (pretty much the
	// same thing
	for _, ld := range ub.UData().Stxos {
		txo := btcdwire.NewTxOut(ld.Amt, ld.PkScript)
		utxo := blockchain.NewUtxoEntry(txo, ld.Height, ld.Coinbase)
		op := btcdwire.OutPoint{
			Hash:  chainhash.Hash(ld.TxHash),
			Index: ld.Index,
		}
		m[op] = utxo
	}

	return v
}
