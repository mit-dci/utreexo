package leaftx

import (
	"crypto/sha256"
	"fmt"

	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"github.com/mit-dci/utreexo/util"
)

// Tx includes everything a CSN node needs to validate a transaction.
// Doesn't include the data for Utreexo inclusion validation.
type Tx struct {
	Version  int32
	TxIn     []*TxIn
	TxOut    []*wire.TxOut
	LockTime uint32
}

// TxIn is a wrapper around wire.TxIn. Includes ValData that's needed for
// leaf commit hashes for Utreexo trees and tx script validation.
type TxIn struct {
	WireTxIn wire.TxIn
	ValData  LeafData
}

// LeafData is all the data that goes into a leaf in the utreexo accumulator
// This is all the info that's needed to verify a bitcoin transaction.
// This acts in place of the utxo set
type LeafData struct {
	BlockHash [32]byte
	Outpoint  wire.OutPoint
	Height    int32
	Coinbase  bool
	Amt       int64
	PkScript  []byte
}

func (tx *Tx) ToBtcUtilTx() *btcutil.Tx {
	var wiretxin []*wire.TxIn
	for _, leaftxin := range tx.TxIn {
		wiretxin = append(wiretxin, &leaftxin.WireTxIn)
	}
	msgtx := wire.MsgTx{Version: tx.Version, TxIn: wiretxin, TxOut: tx.TxOut}
	return btcutil.NewTx(&msgtx)
}

// turn a LeafData into a LeafHash
func (l *LeafData) LeafHash() [32]byte {
	return sha256.Sum256(l.ToBytes())
}

// turn a LeafData into bytes
func (l *LeafData) ToString() (s string) {
	s = l.Outpoint.String()
	s += fmt.Sprintf(" bh %x ", l.BlockHash)
	s += fmt.Sprintf("h %d ", l.Height)
	s += fmt.Sprintf("cb %v ", l.Coinbase)
	s += fmt.Sprintf("amt %d ", l.Amt)
	s += fmt.Sprintf("pks %x ", l.PkScript)
	s += fmt.Sprintf("%x", l.LeafHash())
	return
}

// compact serialization for LeafData:
// don't need to send BlockHash; figure it out from height
// don't need to send outpoint, it's already in the msgBlock
// can use tags for PkScript
// so it's just height, coinbaseness, amt, pkscript tag

// turn a LeafData into bytes (compact, for sending in blockProof) -
// don't hash this, it doesn't commit to everything
func (l *LeafData) ToCompactBytes() (b []byte) {
	l.Height <<= 1
	if l.Coinbase {
		l.Height |= 1
	}
	b = append(b, util.I32tB(l.Height)...)
	b = append(b, util.I64tB(l.Amt)...)
	b = append(b, l.PkScript...)
	return
}

// turn a LeafData into bytes
func (l *LeafData) ToBytes() (b []byte) {
	b = append(l.BlockHash[:], l.Outpoint.Hash[:]...)
	b = append(b, util.U32tB(l.Outpoint.Index)...)
	hcb := l.Height << 1
	if l.Coinbase {
		hcb |= 1
	}
	b = append(b, util.I32tB(hcb)...)
	b = append(b, util.I64tB(l.Amt)...)
	b = append(b, l.PkScript...)
	return
}

func LeafDataFromBytes(b []byte) (LeafData, error) {
	var l LeafData
	if len(b) < 80 {
		return l, fmt.Errorf("Not long enough for leafdata, need 80 bytes")
	}
	//copy(l.BlockHash[:], b[0:32])
	copy(l.Outpoint.Hash[:], b[32:64])
	l.Outpoint.Index = util.BtU32(b[64:68])
	l.Height = util.BtI32(b[68:72])
	if l.Height&1 == 1 {
		l.Coinbase = true
	}
	l.Height >>= 1
	l.Amt = util.BtI64(b[72:80])
	l.PkScript = b[80:]

	return l, nil
}

// LeafDataFromCompactBytes doesn't fill in blockhash, outpoint, and in
// most cases PkScript, so something else has to fill those in later.
func LeafDataFromCompactBytes(b []byte) (LeafData, error) {
	var l LeafData
	if len(b) < 13 {
		return l, fmt.Errorf("Not long enough for leafdata, need 80 bytes")
	}
	l.Height = util.BtI32(b[0:4])
	if l.Height&1 == 1 {
		l.Coinbase = true
	}
	l.Height >>= 1
	l.Amt = util.BtI64(b[4:12])
	l.PkScript = b[12:]

	return l, nil
}

func LeafDataFromTxo(txo wire.TxOut) (LeafData, error) {
	var l LeafData

	return l, nil
}
