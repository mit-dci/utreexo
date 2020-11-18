package util

import (
	"fmt"
	"sync"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

// ProofsProveBlock checks the consistency of a UBlock.  Does the proof prove
// all the inputs in the block?
func (ub *UBlock) ProofsProveBlock(inputSkipList []uint32) bool {
	// get the outpoints that need proof
	proveOPs := BlockToDelOPs(&ub.Block, inputSkipList)

	// ensure that all outpoints are provided in the extradata
	if len(proveOPs) != len(ub.UtreexoData.Stxos) {
		fmt.Printf("height %d %d outpoints need proofs but only %d proven\n",
			ub.UtreexoData.Height, len(proveOPs), len(ub.UtreexoData.Stxos))
		return false
	}
	for i, _ := range ub.UtreexoData.Stxos {
		if proveOPs[i] != ub.UtreexoData.Stxos[i].Outpoint {
			fmt.Printf("block/utxoData mismatch %s v %s\n",
				proveOPs[i].String(), ub.UtreexoData.Stxos[i].Outpoint.String())
			return false
		}
	}
	return true
}

// Verify checks the consistency of uData: that the utxos are proven in the
// batchproof
func (ud *UData) Verify(nl uint64, h uint8) bool {

	// this is really ugly and basically copies the whole thing to avoid
	// destroying it while verifying...

	// presort := make([]uint64, len(ud.AccProof.Targets))
	// copy(presort, ud.AccProof.Targets)

	// fmt.Printf(ud.AccProof.ToString())

	// ud.AccProof.SortTargets()
	mp, err := ud.AccProof.Reconstruct(nl, h)
	if err != nil {
		fmt.Printf("Reconstruct failed %s\n", err.Error())
		return false
	}

	// make sure the udata is consistent, with the same number of leafDatas
	// as targets in the accumulator batch proof
	if len(ud.AccProof.Targets) != len(ud.Stxos) {
		fmt.Printf("Verify failed: %d targets but %d leafdatas\n",
			len(ud.AccProof.Targets), len(ud.Stxos))
	}

	for i, pos := range ud.AccProof.Targets {
		hashInProof, exists := mp[pos]
		if !exists {
			fmt.Printf("Verify failed: Target %d not in map\n", pos)
			return false
		}
		// check if leafdata hashes to the hash in the proof at the target
		if ud.Stxos[i].LeafHash() != hashInProof {
			fmt.Printf("Verify failed: txo %s pos %d leafdata %x in proof %x\n",
				ud.Stxos[i].Outpoint.String(), pos,
				ud.Stxos[i].LeafHash(), hashInProof)
			sib, exists := mp[pos^1]
			if exists {
				fmt.Printf("sib exists, %x\n", sib)
			}
			return false
		}
	}
	// return to presorted target list
	// ud.AccProof.Targets = presort
	return true
}

// ToUtxoView converts a UData into a btcd blockchain.UtxoViewpoint
// all the data is there, just a bit different format.
// Note that this needs blockchain.NewUtxoEntry() in btcd
func (ud *UData) ToUtxoView() *blockchain.UtxoViewpoint {
	v := blockchain.NewUtxoViewpoint()
	m := v.Entries()
	// loop through leafDatas and convert them into UtxoEntries (pretty much the
	// same thing
	for _, ld := range ud.Stxos {
		txo := wire.NewTxOut(ld.Amt, ld.PkScript)
		utxo := blockchain.NewUtxoEntry(txo, ld.Height, ld.Coinbase)
		m[ld.Outpoint] = utxo
	}

	return v
}

/*
blockchain.NewUtxoEntry() looks like this:
// NewUtxoEntry returns a new UtxoEntry built from the arguments.
func NewUtxoEntry(
	txOut *wire.TxOut, blockHeight int32, isCoinbase bool) *UtxoEntry {
	var cbFlag txoFlags
	if isCoinbase {
		cbFlag |= tfCoinBase
	}

	return &UtxoEntry{
		amount:      txOut.Value,
		pkScript:    txOut.PkScript,
		blockHeight: blockHeight,
		packedFlags: cbFlag,
	}
}
*/

// CheckBlock does all internal block checks for a UBlock
// right now checks the signatures
func (ub *UBlock) CheckBlock(outskip []uint32, p *chaincfg.Params) bool {
	// NOTE Whatever happens here is done a million times
	// be efficient here
	view := ub.UtreexoData.ToUtxoView()
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
