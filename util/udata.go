package util

import (
	"fmt"

	"github.com/btcsuite/btcd/chaincfg"

	"github.com/btcsuite/btcutil"

	"github.com/btcsuite/btcd/blockchain"
)

// ProofsProveBlock checks the consistency of a UBlock.  Does the proof prove
// all the inputs in the block?
func (ub *UBlock) ProofsProveBlock(inputSkipList []uint32) bool {
	// get the outpoints that need proof
	proveOPs := blockToDelOPs(&ub.Block, inputSkipList)

	// ensure that all outpoints are provided in the extradata
	if len(proveOPs) != len(ub.ExtraData.UtxoData) {
		fmt.Printf("%d outpoints need proofs but only %d proven\n",
			len(proveOPs), len(ub.ExtraData.UtxoData))
		return false
	}
	for i, _ := range ub.ExtraData.UtxoData {
		if proveOPs[i] != ub.ExtraData.UtxoData[i].Outpoint {
			fmt.Printf("block/utxoData mismatch %s v %s\n",
				proveOPs[i].String(), ub.ExtraData.UtxoData[i].Outpoint.String())
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

	presort := make([]uint64, len(ud.AccProof.Targets))
	copy(presort, ud.AccProof.Targets)

	ud.AccProof.SortTargets()
	mp, err := ud.AccProof.Reconstruct(nl, h)
	if err != nil {
		fmt.Printf(" Reconstruct failed %s\n", err.Error())
		return false
	}

	// make sure the udata is consistent, with the same number of leafDatas
	// as targets in the accumulator batch proof
	if len(ud.AccProof.Targets) != len(ud.UtxoData) {
		fmt.Printf("Verify failed: %d targets but %d leafdatas\n",
			len(ud.AccProof.Targets), len(ud.UtxoData))
	}

	for i, pos := range presort {
		hashInProof, exists := mp[pos]
		if !exists {
			fmt.Printf("Verify failed: Target %d not in map\n", pos)
			return false
		}
		// check if leafdata hashes to the hash in the proof at the target
		if ud.UtxoData[i].LeafHash() != hashInProof {
			fmt.Printf("Verify failed: txo %s position %d leafdata %x proof %x\n",
				ud.UtxoData[i].Outpoint.String(), pos,
				ud.UtxoData[i].LeafHash(), hashInProof)
			sib, exists := mp[pos^1]
			if exists {
				fmt.Printf("sib exists, %x\n", sib)
			}
			return false
		}
	}
	// return to presorted target list
	ud.AccProof.Targets = presort
	return true
}

func (ud *UData) ToUtxoView() *blockchain.UtxoViewpoint {
	v := blockchain.NewUtxoViewpoint()
	m := v.Entries()
	// loop through leafDatas and convert them into UtxoEntries (pretty much the
	// same thing
	for _, ld := range ud.UtxoData {
		utxo := blockchain.NewUtxoEntry(
			ld.Amt, ld.PkScript, ld.Height, ld.Coinbase)
		m[ld.Outpoint] = utxo
	}

	return v
}

// CheckBlock does all internal block checks for a UBlock
// right now checks the signatures
func (ub *UBlock) CheckBlock() bool {

	view := ub.ExtraData.ToUtxoView()

	for _, tx := range ub.Block.Transactions {
		utilTx := btcutil.NewTx(tx)
		// hardcoded testnet3 for now
		_, err := blockchain.CheckTransactionInputs(
			utilTx, ub.Height, view, &chaincfg.TestNet3Params)
		if err != nil {
			fmt.Printf("Tx %s fails CheckTransactionInputs: %s\n",
				utilTx.Hash().String(), err.Error())
			return false
		}
	}

	return true
}
