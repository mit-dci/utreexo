// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.
package consensus

import (
	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"github.com/mit-dci/utreexo/util"
)

// ValidateTx validates a tx per consensus rules for the Utreexo CSN
// Does not validate if a tx is a standard tx (as in, it follows the policy
// rules)
func ValidateTx(tx *btcutil.Tx, txheight int32, utxoView *blockchain.UtxoViewpoint) error {
	// Perform preliminary sanity checks on the transaction. This makes
	// use of blockchain which contains the invariant rules for what
	// transactions are allowed into blocks.
	err := blockchain.CheckTransactionSanity(tx)
	if err != nil {
		return (err)
	}

	// A standalone transaction must not be a coinbase transaction.
	if blockchain.IsCoinBase(tx) {
		return (err)
	}

	param := chaincfg.TestNet3Params
	_, err = blockchain.CheckTransactionInputs(tx, txheight,
		utxoView, &param)
	if err != nil {
		return (err)
	}

	// NOTE: if you modify this code to accept non-standard transactions,
	// you should add code here to check that the transaction does a
	// reasonable number of ECDSA signature verifications.
	// TODO(kcalvinalvin): idk I guess we don't care about this for now
	// No one is gonna attack a Utreexo node but I guess this is something
	// to keep in mind for the future.

	// Don't allow transactions with an excessive number of signature
	// operations which would result in making it impossible to mine.  Since
	// the coinbase address itself can contain signature operations, the
	// maximum allowed signature operations per transaction is less than
	// the maximum allowed signature operations per block.
	// TODO(roasbeef): last bool should be conditional on segwit activation
	_, err = blockchain.GetSigOpCost(tx, false, utxoView, true, true)
	if err != nil {
		return (err)
	}

	var sigCache txscript.SigCache
	var hashCache txscript.HashCache
	// Verify crypto signatures for each input and reject the transaction if
	// any don't verify.
	err = blockchain.ValidateTransactionScripts(tx, utxoView,
		txscript.StandardVerifyFlags, &sigCache, &hashCache)
	if err != nil {
		return (err)
	}

	return nil
}

// MakePseudoViewPoint makes a UtxoViewpoint containing only the utxos being
// spent in the given block
func makePseudoViewPoint(ub util.UBlock) *blockchain.UtxoViewpoint {
	vp := blockchain.NewUtxoViewpoint()
	var txs []*btcutil.Tx
	for _, utxo := range ub.ExtraData.UtxoData {
		var msgTx wire.MsgTx
		wireOut := wire.TxOut{Value: utxo.Amt,
			PkScript: utxo.PkScript}
		msgTx.TxOut = append(msgTx.TxOut, &wireOut)
		txs = append(txs, btcutil.NewTx(&msgTx))
	}

	for _, tx := range txs {
		vp.AddTxOuts(tx, ub.Height)
	}

	return vp
}

// ValidateUBlock validates the entire txs within the given block
// Does not check if the txs exist
func ValidateUBlock(ub util.UBlock) error {
	vp := makePseudoViewPoint(ub)
	for _, msgTx := range ub.Block.Transactions {
		tx := btcutil.NewTx(msgTx)
		if blockchain.IsCoinBase(tx) {
			continue
		}
		err := ValidateTx(tx, ub.Height, vp)
		if err != nil {
			panic(err)
		}
	}
	return nil
}
