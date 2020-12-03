package wire

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net"
	"sync"
	"time"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"github.com/mit-dci/utreexo/accumulator"
	"github.com/mit-dci/utreexo/btcacc"
	"github.com/mit-dci/utreexo/util"
)

// UblockNetworkReader gets Ublocks from the remote host and puts em in the
// channel.  It'll try to fill the channel buffer.
func UblockNetworkReader(
	blockChan chan UBlock, remoteServer string,
	curHeight, lookahead int32) {

	d := net.Dialer{Timeout: 2 * time.Second}
	con, err := d.Dial("tcp", remoteServer)
	if err != nil {
		panic(err)
	}
	defer con.Close()
	defer close(blockChan)

	var ub UBlock
	// var ublen uint32
	// request range from curHeight to latest block
	err = binary.Write(con, binary.BigEndian, curHeight)
	if err != nil {
		e := fmt.Errorf("UblockNetworkReader: write error to connection %s %s\n",
			con.RemoteAddr().String(), err.Error())
		panic(e)
	}
	err = binary.Write(con, binary.BigEndian, int32(math.MaxInt32))
	if err != nil {
		e := fmt.Errorf("UblockNetworkReader: write error to connection %s %s\n",
			con.RemoteAddr().String(), err.Error())
		panic(e)
	}

	// TODO goroutines for only the Deserialize part might be nice.
	// Need to sort the blocks though if you're doing that
	for ; ; curHeight++ {

		err = ub.Deserialize(con)
		if err != nil {
			fmt.Printf("Deserialize error from connection %s %s\n",
				con.RemoteAddr().String(), err.Error())
			return
		}

		blockChan <- ub
	}
}

// BlockToAdds turns all the new utxos in a msgblock into leafTxos
// uses remember slice up to number of txos, but doesn't check that it's the
// right length.  Similar with skiplist, doesn't check it.
func BlockToAddLeaves(blk wire.MsgBlock,
	remember []bool, skiplist []uint32,
	height int32) (leaves []accumulator.Leaf) {

	var txonum uint32
	// bh := bl.Blockhash
	for coinbaseif0, tx := range blk.Transactions {
		// cache txid aka txhash
		txid := tx.TxHash()
		for i, out := range tx.TxOut {
			// Skip all the OP_RETURNs
			if util.IsUnspendable(out) {
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
			l.TxHash = btcacc.Hash(txid)
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
			// fmt.Printf("add %s\n", l.ToString())
			// fmt.Printf("add %s -> %x\n", l.Outpoint.String(), l.LeafHash())
			txonum++
		}
	}
	return
}

// UBlock is a regular block, with Udata stuck on
type UBlock struct {
	UtreexoData btcacc.UData
	Block       wire.MsgBlock
}

// ProofSanity checks the consistency of a UBlock.  Does the proof prove
// all the inputs in the block?
func (ub *UBlock) ProofSanity(inputSkipList []uint32, nl uint64, h uint8) error {
	// get the outpoints that need proof
	proveOPs := util.BlockToDelOPs(&ub.Block, inputSkipList)

	// ensure that all outpoints are provided in the extradata
	if len(proveOPs) != len(ub.UtreexoData.Stxos) {
		err := fmt.Errorf("height %d %d outpoints need proofs but only %d proven\n",
			ub.UtreexoData.Height, len(proveOPs), len(ub.UtreexoData.Stxos))
		return err
	}
	for i, _ := range ub.UtreexoData.Stxos {
		if btcacc.Hash(proveOPs[i].Hash) != ub.UtreexoData.Stxos[i].TxHash ||
			proveOPs[i].Index != ub.UtreexoData.Stxos[i].Index {
			err := fmt.Errorf("block/utxoData mismatch %s v %s\n",
				proveOPs[i].String(), ub.UtreexoData.Stxos[i].OPString())
			return err
		}
	}
	// derive leafHashes from leafData
	if !ub.UtreexoData.ProofSanity(nl, h) {
		return fmt.Errorf("height %d LeafData / Proof mismatch", ub.UtreexoData.Height)
	}

	return nil
}

// ToUtxoView converts a UData into a btcd blockchain.UtxoViewpoint
// all the data is there, just a bit different format.
// Note that this needs blockchain.NewUtxoEntry() in btcd
func (ub *UBlock) ToUtxoView() *blockchain.UtxoViewpoint {
	v := blockchain.NewUtxoViewpoint()
	m := v.Entries()
	// loop through leafDatas and convert them into UtxoEntries (pretty much the
	// same thing
	for _, ld := range ub.UtreexoData.Stxos {
		txo := wire.NewTxOut(ld.Amt, ld.PkScript)
		utxo := blockchain.NewUtxoEntry(txo, ld.Height, ld.Coinbase)
		op := wire.OutPoint{
			Hash:  chainhash.Hash(ld.TxHash),
			Index: ld.Index,
		}
		m[op] = utxo
	}

	return v
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

	for txnum, tx := range ub.Block.Transactions {
		outputsInTx := uint32(len(tx.TxOut))
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
			skipTxo := wire.NewTxOut(tx.TxOut[idx].Value, tx.TxOut[idx].PkScript)
			skippedEntry := blockchain.NewUtxoEntry(
				skipTxo, ub.UtreexoData.Height, false)
			skippedOutpoint := wire.OutPoint{Hash: tx.TxHash(), Index: idx}
			viewMap[skippedOutpoint] = skippedEntry
			outskip = outskip[1:] // pop off from output skiplist
		}
		txonum += outputsInTx
	}

	var wg sync.WaitGroup
	wg.Add(len(ub.Block.Transactions) - 1) // subtract coinbase
	for txnum, tx := range ub.Block.Transactions {
		if txnum == 0 {
			continue // skip checks for coinbase TX for now.  Or maybe it'll work?
		}
		utilTx := btcutil.NewTx(tx)
		go func(w *sync.WaitGroup, tx *btcutil.Tx) {
			// hardcoded testnet3 for now
			_, err := blockchain.CheckTransactionInputs(
				utilTx, ub.UtreexoData.Height, view, p)
			if err != nil {
				fmt.Printf("Tx %s fails CheckTransactionInputs: %s\n",
					utilTx.Hash().String(), err.Error())
				panic(err)
			}

			// no scriptflags for now
			err = blockchain.ValidateTransactionScripts(
				utilTx, view, 0, sigCache, hashCache)
			if err != nil {
				fmt.Printf("Tx %s fails ValidateTransactionScripts: %s\n",
					utilTx.Hash().String(), err.Error())
				panic(err)
			}
			w.Done()
		}(&wg, utilTx)
	}
	wg.Wait()

	return true
}

/*
Ublock serialization
(changed with flatttl branch)

A "Ublock" is a regular bitcoin block, along with Utreexo-specific data.
The udata comes first, and the height and leafTTLs come first.

*/

// Deserialize a UBlock.  It's just a block then udata.
func (ub *UBlock) Deserialize(r io.Reader) (err error) {
	err = ub.Block.Deserialize(r)
	if err != nil {
		return err
	}
	err = ub.UtreexoData.Deserialize(r)
	return
}

// We don't actually call serialize since from the server side we don't
// serialize, we just glom stuff together from the disk and send it over.
func (ub *UBlock) Serialize(w io.Writer) (err error) {
	err = ub.Block.Serialize(w)
	if err != nil {
		return
	}
	err = ub.UtreexoData.Serialize(w)
	return
}

// SerializeSize: how big is it, in bytes.
func (ub *UBlock) SerializeSize() int {
	return ub.Block.SerializeSize() + ub.UtreexoData.SerializeSize()
}
