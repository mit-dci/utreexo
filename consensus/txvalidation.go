// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.
package consensus

import (
	"fmt"
	"math"
	"runtime"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"github.com/mit-dci/utreexo/util"
)

// txValidateItem holds a transaction along with which input to validate.
type txValidateItem struct {
	txInIndex int
	txIn      *wire.TxIn
	tx        *btcutil.Tx
	sigHashes *txscript.TxSigHashes
}

// txValidator provides a type which asynchronously validates transaction
// inputs.  It provides several channels for communication and a processing
// function that is intended to be in run multiple goroutines.
type txValidator struct {
	validateChan chan *txValidateItem
	quitChan     chan struct{}
	resultChan   chan error
	flags        txscript.ScriptFlags
	ld           util.LeafData
	sigCache     *txscript.SigCache
	hashCache    *txscript.HashCache
}

// sendResult sends the result of a script pair validation on the internal
// result channel while respecting the quit channel.  This allows orderly
// shutdown when the validation process is aborted early due to a validation
// error in one of the other goroutines.
func (v *txValidator) sendResult(result error) {
	select {
	case v.resultChan <- result:
	case <-v.quitChan:
	}
}

// validateHandler consumes items to validate from the internal validate channel
// and returns the result of the validation on the internal result channel. It
// must be run as a goroutine.
func (v *txValidator) validateHandler() {
out:
	for {
		select {
		case txVI := <-v.validateChan:
			// Ensure the referenced input utxo is available.
			txIn := txVI.txIn

			// Create a new script engine for the script pair.
			sigScript := txIn.SignatureScript
			witness := txIn.Witness
			pkScript := v.ld.PkScript
			inputAmount := v.ld.Amt
			vm, err := txscript.NewEngine(pkScript, txVI.tx.MsgTx(),
				txVI.txInIndex, v.flags, v.sigCache, txVI.sigHashes,
				inputAmount)
			if err != nil {
				str := fmt.Sprintf("failed to parse input "+
					"%s:%d which references output %v - "+
					"%v (input witness %x, input script "+
					"bytes %x, prev output script bytes %x)",
					txVI.tx.Hash(), txVI.txInIndex,
					txIn.PreviousOutPoint, err, witness,
					sigScript, pkScript)
				err := fmt.Errorf(str)
				v.sendResult(err)
				break out
			}

			// Execute the script pair.
			if err := vm.Execute(); err != nil {
				str := fmt.Sprintf("failed to validate input "+
					"%s:%d which references output %v - "+
					"%v (input witness %x, input script "+
					"bytes %x, prev output script bytes %x)",
					txVI.tx.Hash(), txVI.txInIndex,
					txIn.PreviousOutPoint, err, witness,
					sigScript, pkScript)
				err := fmt.Errorf(str)
				v.sendResult(err)
				break out
			}

			// Validation succeeded.
			v.sendResult(nil)

		case <-v.quitChan:
			break out
		}
	}
}

// newTxValidator returns a new instance of txValidator to be used for
// validating transaction scripts asynchronously.
func newTxValidator(flags txscript.ScriptFlags, ld util.LeafData,
	sigCache *txscript.SigCache, hashCache *txscript.HashCache) *txValidator {
	return &txValidator{
		validateChan: make(chan *txValidateItem),
		quitChan:     make(chan struct{}),
		resultChan:   make(chan error),
		ld:           ld,
		sigCache:     sigCache,
		hashCache:    hashCache,
		flags:        flags,
	}
}

// Validate validates the scripts for all of the passed transaction inputs using
// multiple goroutines.
func (v *txValidator) Validate(items []*txValidateItem) error {
	if len(items) == 0 {
		return nil
	}

	// Limit the number of goroutines to do script validation based on the
	// number of processor cores.  This helps ensure the system stays
	// reasonably responsive under heavy load.
	maxGoRoutines := runtime.NumCPU() * 3
	if maxGoRoutines <= 0 {
		maxGoRoutines = 1
	}
	if maxGoRoutines > len(items) {
		maxGoRoutines = len(items)
	}

	// Start up validation handlers that are used to asynchronously
	// validate each transaction input.
	for i := 0; i < maxGoRoutines; i++ {
		go v.validateHandler()
	}

	// Validate each of the inputs.  The quit channel is closed when any
	// errors occur so all processing goroutines exit regardless of which
	// input had the validation error.
	numInputs := len(items)
	currentItem := 0
	processedItems := 0
	for processedItems < numInputs {
		// Only send items while there are still items that need to
		// be processed.  The select statement will never select a nil
		// channel.
		var validateChan chan *txValidateItem
		var item *txValidateItem
		if currentItem < numInputs {
			validateChan = v.validateChan
			item = items[currentItem]
		}

		select {
		case validateChan <- item:
			currentItem++

		case err := <-v.resultChan:
			processedItems++
			if err != nil {
				close(v.quitChan)
				return err
			}
		}
	}

	close(v.quitChan)
	return nil
}

// ValidateTransactionScripts validates the scripts for the passed transaction
// using multiple goroutines.
func ValidateTransactionScripts(tx *btcutil.Tx, ld util.LeafData,
	flags txscript.ScriptFlags, sigCache *txscript.SigCache,
	hashCache *txscript.HashCache) error {

	// First determine if segwit is active according to the scriptFlags. If
	// it isn't then we don't need to interact with the HashCache.
	segwitActive := flags&txscript.ScriptVerifyWitness == txscript.ScriptVerifyWitness

	// If the hashcache doesn't yet has the sighash midstate for this
	// transaction, then we'll compute them now so we can re-use them
	// amongst all worker validation goroutines.
	if segwitActive && tx.MsgTx().HasWitness() &&
		!hashCache.ContainsHashes(tx.Hash()) {
		hashCache.AddSigHashes(tx.MsgTx())
	}

	var cachedHashes *txscript.TxSigHashes
	if segwitActive && tx.MsgTx().HasWitness() {
		// The same pointer to the transaction's sighash midstate will
		// be re-used amongst all validation goroutines. By
		// pre-computing the sighash here instead of during validation,
		// we ensure the sighashes
		// are only computed once.
		cachedHashes, _ = hashCache.GetSigHashes(tx.Hash())
	}

	// Collect all of the transaction inputs and required information for
	// validation.
	txIns := tx.MsgTx().TxIn
	txValItems := make([]*txValidateItem, 0, len(txIns))
	for txInIdx, txIn := range txIns {
		// Skip coinbases.
		if txIn.PreviousOutPoint.Index == math.MaxUint32 {
			continue
		}

		txVI := &txValidateItem{
			txInIndex: txInIdx,
			txIn:      txIn,
			tx:        tx,
			sigHashes: cachedHashes,
		}
		txValItems = append(txValItems, txVI)
	}

	// Validate all of the inputs.
	validator := newTxValidator(flags, ld, sigCache, hashCache)
	return validator.Validate(txValItems)
}

// CountP2SHSigOps returns the number of signature operations for all input
// transactions which are of the pay-to-script-hash type.  This uses the
// precise, signature operation counting mechanism from the script engine which
// requires access to the input transaction scripts.
func CountP2SHSigOps(tx *btcutil.Tx, ld util.LeafData) (int, error) {
	// Coinbase transactions have no interesting inputs.
	if ld.Coinbase {
		return 0, nil
	}

	// Accumulate the number of signature operations in all transaction
	// inputs.
	msgTx := tx.MsgTx()
	totalSigOps := 0
	for _, txIn := range msgTx.TxIn {

		// We're only interested in pay-to-script-hash types, so skip
		// this input if it's not one.
		pkScript := ld.PkScript
		if !txscript.IsPayToScriptHash(pkScript) {
			continue
		}

		// Count the precise number of signature operations in the
		// referenced public key script.
		sigScript := txIn.SignatureScript
		numSigOps := txscript.GetPreciseSigOpCount(sigScript, pkScript,
			true)

		// We could potentially overflow the accumulator so check for
		// overflow.
		lastSigOps := totalSigOps
		totalSigOps += numSigOps
		if totalSigOps < lastSigOps {
			str := fmt.Sprintf("the public key script from output "+
				"%v contains too many signature operations - "+
				"overflow", txIn.PreviousOutPoint)
			return 0, fmt.Errorf(str)
		}
	}

	return totalSigOps, nil
}

// GetSigOpCost returns the unified sig op cost for the passed transaction
// respecting current active soft-forks which modified sig op cost counting.
// The unified sig op cost for a transaction is computed as the sum of: the
// legacy sig op count scaled according to the WitnessScaleFactor, the sig op
// count for all p2sh inputs scaled by the WitnessScaleFactor, and finally the
// unscaled sig op count for any inputs spending witness programs.
func GetSigOpCost(tx *btcutil.Tx, ld util.LeafData, bip16, segWit bool) (int, error) {
	numSigOps := blockchain.CountSigOps(tx) * blockchain.WitnessScaleFactor
	if bip16 {
		numP2SHSigOps, err := CountP2SHSigOps(tx, ld)
		if err != nil {
			return 0, nil
		}
		numSigOps += (numP2SHSigOps * blockchain.WitnessScaleFactor)
	}

	if segWit && !ld.Coinbase {
		msgTx := tx.MsgTx()
		for _, txIn := range msgTx.TxIn {
			/*
				// Ensure the referenced output is available and hasn't
				// already been spent.
				utxo := utxoView.LookupEntry(txIn.PreviousOutPoint)
				if utxo == nil || utxo.IsSpent() {
					str := fmt.Sprintf("output %v referenced from "+
						"transaction %s:%d either does not "+
						"exist or has already been spent",
						txIn.PreviousOutPoint, tx.Hash(),
						txInIndex)
					return 0, fmt.Errorf(str)
				}
			*/

			witness := txIn.Witness
			sigScript := txIn.SignatureScript
			pkScript := ld.PkScript
			numSigOps += txscript.GetWitnessSigOpCount(sigScript, pkScript, witness)
		}

	}

	return numSigOps, nil
}

// CheckTransactionInputs performs a series of checks on the inputs to a
// transaction to ensure they are valid.  An example of some of the checks
// include verifying all inputs exist, ensuring the coinbase seasoning
// requirements are met, detecting double spends, validating all values and fees
// are in the legal range and the total output amount doesn't exceed the input
// amount, and verifying the signatures to prove the spender was the owner of
// the bitcoins and therefore allowed to spend them.  As it checks the inputs,
// it also calculates the total fees for the transaction and returns that value.
//
// NOTE: The transaction MUST have already been sanity checked with the
// CheckTransactionSanity function prior to calling this function.
func CheckTransactionInputs(tx *btcutil.Tx, ld util.LeafData, txHeight int32,
	chainParams *chaincfg.Params) (int64, error) {
	// Coinbase transactions have no inputs.
	if blockchain.IsCoinBase(tx) {
		return 0, nil
	}

	txHash := tx.Hash()
	var totalSatoshiIn int64
	for _, txIn := range tx.MsgTx().TxIn {
		/*
			// Ensure the referenced input transaction is available.
			utxo := utxoView.LookupEntry(txIn.PreviousOutPoint)
			if utxo == nil || utxo.IsSpent() {
				str := fmt.Sprintf("output %v referenced from "+
					"transaction %s:%d either does not exist or "+
					"has already been spent", txIn.PreviousOutPoint,
					tx.Hash(), txInIndex)
				return 0, ruleError(ErrMissingTxOut, str)
			}
		*/

		// Ensure the transaction is not spending coins which have not
		// yet reached the required coinbase maturity.
		if ld.Coinbase {
			originHeight := ld.Height
			blocksSincePrev := txHeight - originHeight
			coinbaseMaturity := int32(chainParams.CoinbaseMaturity)
			if blocksSincePrev < coinbaseMaturity {
				str := fmt.Sprintf("tried to spend coinbase "+
					"transaction output %v from height %v "+
					"at height %v before required maturity "+
					"of %v blocks", txIn.PreviousOutPoint,
					originHeight, txHeight,
					coinbaseMaturity)
				return 0, fmt.Errorf(str)
			}
		}

		// Ensure the transaction amounts are in range.  Each of the
		// output values of the input transactions must not be negative
		// or more than the max allowed per transaction.  All amounts in
		// a transaction are in a unit value known as a satoshi.  One
		// bitcoin is a quantity of satoshi as defined by the
		// SatoshiPerBitcoin constant.
		originTxSatoshi := ld.Amt
		if originTxSatoshi < 0 {
			str := fmt.Sprintf("transaction output has negative "+
				"value of %v", btcutil.Amount(originTxSatoshi))
			return 0, fmt.Errorf(str)
		}
		if originTxSatoshi > btcutil.MaxSatoshi {
			str := fmt.Sprintf("transaction output value of %v is "+
				"higher than max allowed value of %v",
				btcutil.Amount(originTxSatoshi),
				btcutil.MaxSatoshi)
			return 0, fmt.Errorf(str)
		}

		// The total of all outputs must not be more than the max
		// allowed per transaction.  Also, we could potentially overflow
		// the accumulator so check for overflow.
		lastSatoshiIn := totalSatoshiIn
		totalSatoshiIn += originTxSatoshi
		if totalSatoshiIn < lastSatoshiIn ||
			totalSatoshiIn > btcutil.MaxSatoshi {
			str := fmt.Sprintf("total value of all transaction "+
				"inputs is %v which is higher than max "+
				"allowed value of %v", totalSatoshiIn,
				btcutil.MaxSatoshi)
			return 0, fmt.Errorf(str)
		}
	}

	// Calculate the total output amount for this transaction.  It is safe
	// to ignore overflow and out of range errors here because those error
	// conditions would have already been caught by checkTransactionSanity.
	var totalSatoshiOut int64
	for _, txOut := range tx.MsgTx().TxOut {
		totalSatoshiOut += txOut.Value
	}

	// Ensure the transaction does not spend more than its inputs.
	if totalSatoshiIn < totalSatoshiOut {
		str := fmt.Sprintf("total value of all transaction inputs for "+
			"transaction %v is %v which is less than the amount "+
			"spent of %v", txHash, totalSatoshiIn, totalSatoshiOut)
		return 0, fmt.Errorf(str)
	}

	// NOTE: bitcoind checks if the transaction fees are < 0 here, but that
	// is an impossible condition because of the check above that ensures
	// the inputs are >= the outputs.
	txFeeInSatoshi := totalSatoshiIn - totalSatoshiOut
	return txFeeInSatoshi, nil
}

// ValidateTx validates a tx per consensus rules for the Utreexo CSN
// Does not validate if a tx is a standard tx (as in, it follows the policy
// rules)
func ValidateTx(tx *btcutil.Tx, ld util.LeafData, txheight int32) error {
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
	_, err = CheckTransactionInputs(tx, ld, txheight, &param)
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
	_, err = GetSigOpCost(tx, ld, true, true)
	if err != nil {
		return (err)
	}

	var sigCache txscript.SigCache
	var hashCache txscript.HashCache
	// Verify crypto signatures for each input and reject the transaction if
	// any don't verify.
	err = ValidateTransactionScripts(tx, ld,
		txscript.StandardVerifyFlags, &sigCache, &hashCache)
	if err != nil {
		return (err)
	}

	return nil
}

// ValidateUBlock validates the entire txs within the given block
// Does not check if the txs exist
func ValidateUBlock(ub util.UBlock) error {
	for idx, msgTx := range ub.Block.Transactions {
		tx := btcutil.NewTx(msgTx)
		if blockchain.IsCoinBase(tx) {
			continue
		}
		err := ValidateTx(tx, ub.ExtraData.UtxoData[idx], ub.Height)
		if err != nil {
			panic(err)
		}
	}
	return nil
}
