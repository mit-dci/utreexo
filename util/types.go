package util

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/btcsuite/btcd/wire"
	"github.com/mit-dci/utreexo/accumulator"
)

type Hash [32]byte

// HashFromString hashes the given string with sha256
func HashFromString(s string) Hash {
	return sha256.Sum256([]byte(s))
}

type ProofAndHeight struct {
	Proof  []byte
	Height int32
}

// Tx defines a bitcoin transaction that provides easier and more efficient
// manipulation of raw transactions.  It also memoizes the hash for the
// transaction on its first access so subsequent accesses don't have to repeat
// the relatively expensive hashing operations.
type zProofTx struct {
	msgTx         *wire.MsgTx // Underlying MsgTx
	txHash        *Hash       // Cached transaction hash
	txHashWitness *Hash       // Cached transaction witness hash
	txHasWitness  *bool       // If the transaction has witness data
	txIndex       int         // Position within a block or TxIndexUnknown
}

// UBlock is a regular block, with Udata stuck on
type UBlock struct {
	Block     wire.MsgBlock
	ExtraData UData
	Height    int32
}

type UData struct {
	AccProof accumulator.BatchProof
	UtxoData []LeafData
	LeafTTLs []uint32
}

// LeafData is all the data that goes into a leaf in the utreexo accumulator
type LeafData struct {
	BlockHash [32]byte
	Outpoint  wire.OutPoint
	Height    int32
	Coinbase  bool
	Amt       int64
	PkScript  []byte
}

// ToString turns a LeafData into bytes
func (l *LeafData) ToString() (s string) {
	s = l.Outpoint.String()
	// s += fmt.Sprintf(" bh %x ", l.BlockHash)
	s += fmt.Sprintf("h %d ", l.Height)
	s += fmt.Sprintf("cb %v ", l.Coinbase)
	s += fmt.Sprintf("amt %d ", l.Amt)
	s += fmt.Sprintf("pks %x ", l.PkScript)
	s += fmt.Sprintf("%x", l.LeafHash())
	return
}

func LeafDataFromBytes(b []byte) (LeafData, error) {
	var l LeafData
	if len(b) < 80 {
		return l, fmt.Errorf("%x for leafdata, need 80 bytes", b)
	}
	copy(l.BlockHash[:], b[0:32])
	copy(l.Outpoint.Hash[:], b[32:64])
	l.Outpoint.Index = binary.BigEndian.Uint32(b[64:68])
	l.Height = int32(binary.BigEndian.Uint32(b[68:72]))
	if l.Height&1 == 1 {
		l.Coinbase = true
	}
	l.Height >>= 1
	l.Amt = int64(binary.BigEndian.Uint64(b[72:80]))
	l.PkScript = b[80:]

	return l, nil
}

// ToBytes turns LeafData into bytes
func (l *LeafData) ToBytes() []byte {
	hcb := l.Height << 1
	if l.Coinbase {
		hcb |= 1
	}

	var buf bytes.Buffer

	buf.Write(l.BlockHash[:])
	buf.Write(l.Outpoint.Hash[:])
	binary.Write(&buf, binary.BigEndian, l.Outpoint.Index)
	binary.Write(&buf, binary.BigEndian, hcb)
	binary.Write(&buf, binary.BigEndian, l.Amt)
	buf.Write(l.PkScript)

	return buf.Bytes()
}

// compact serialization for LeafData:
// don't need to send BlockHash; figure it out from height
// don't need to send outpoint, it's already in the msgBlock
// can use tags for PkScript
// so it's just height, coinbaseness, amt, pkscript tag

// ToCompactBytes turns a LeafData into bytes
// (compact, for sending in blockProof) - don't hash this,
// it doesn't commit to everything
func (l *LeafData) ToCompactBytes() []byte {
	l.Height <<= 1
	if l.Coinbase {
		l.Height |= 1
	}

	var buf bytes.Buffer

	binary.Write(&buf, binary.BigEndian, l.Height)
	binary.Write(&buf, binary.BigEndian, l.Amt)
	buf.Write(l.PkScript)

	return buf.Bytes()
}

// LeafDataFromCompactBytes doesn't fill in blockhash, outpoint, and in
// most cases PkScript, so something else has to fill those in later.
func LeafDataFromCompactBytes(b []byte) (LeafData, error) {
	var l LeafData
	if len(b) < 13 {
		return l, fmt.Errorf("Not long enough for leafdata, need 80 bytes")
	}
	l.Height = int32(binary.BigEndian.Uint32(b[0:4]))
	if l.Height&1 == 1 {
		l.Coinbase = true
	}
	l.Height >>= 1
	l.Amt = int64(binary.BigEndian.Uint64(b[4:12]))
	l.PkScript = b[12:]

	return l, nil
}

// LeafHash turns a LeafData into a LeafHash
func (l *LeafData) LeafHash() [32]byte {
	return sha256.Sum256(l.ToBytes())
}

func LeafDataFromTxo(txo wire.TxOut) (LeafData, error) {
	var l LeafData

	return l, nil
}

// ToBytes serializes UData into bytes.
// There's a bunch of variable length things (the batchProof.hashes, and the
// LeafDatas) so we prefix lengths for those.  Ordering is:
// batch proof length (4 bytes)
// batch proof
// Bunch of LeafDatas, prefixed with 2-byte lengths
// leaf ttls 4 bytes each
func (ud *UData) ToBytes() []byte {
	batchBytes := ud.AccProof.ToBytes()
	buffer := new(bytes.Buffer)
	// write the length of the batch proof bytes
	binary.Write(buffer, binary.BigEndian, uint32(len(batchBytes)))
	// write the batch proof bytes
	buffer.Write(batchBytes)
	// write the utxos data
	for _, utxo := range ud.UtxoData {
		// write the utxo data prefixed by 2 length bytes
		buffer.Write(PrefixLen16(utxo.ToBytes()))
	}
	// write the number off ttls
	binary.Write(buffer, binary.BigEndian, uint32(len(ud.LeafTTLs)))
	// write the TTL values
	for _, ttl := range ud.LeafTTLs {
		binary.Write(buffer, binary.BigEndian, ttl)
	}

	return buffer.Bytes()
}

// UDataFromBytes deserializes into UData from bytes.
func UDataFromBytes(b []byte) (ud UData, err error) {
	// if there's no bytes, it's an empty uData
	if len(b) == 0 {
		return
	}

	if len(b) < 4 {
		err = fmt.Errorf("block proof too short %d bytes", len(b))
		return
	}

	buffer := bytes.NewBuffer(b)

	// read the length of the batch proof bytes
	var batchLen uint32
	binary.Read(buffer, binary.BigEndian, &batchLen)
	if batchLen > uint32(len(b)-4) {
		err = fmt.Errorf("block proof says %d bytes but %d remain",
			batchLen, len(b)-4)
		return
	}
	// read the batch proof bytes
	ud.AccProof, err = accumulator.FromBytesBatchProof(buffer.Next(int(batchLen)))
	if err != nil {
		return
	}
	// read the utxo datas, there are as many utxos as there are targets in the proof.
	ud.UtxoData = make([]LeafData, len(ud.AccProof.Targets))
	for i, _ := range ud.UtxoData {
		dataSize := binary.BigEndian.Uint16(buffer.Next(2))
		dataBytes := buffer.Next(int(dataSize))
		ud.UtxoData[i], err = LeafDataFromBytes(dataBytes)
		if err != nil {
			return
		}
	}

	if buffer.Len() > 0 {
		ttlCount := binary.BigEndian.Uint32(buffer.Next(4))
		// read the TTL values, there are as many TTLs as there are targets in the proof.
		ud.LeafTTLs = make([]uint32, ttlCount)
		for i, _ := range ud.LeafTTLs {
			// each ttl is 4 bytes (uint32)
			ud.LeafTTLs[i] = binary.BigEndian.Uint32(buffer.Next(4))
		}
	}

	return ud, nil
}

func (ub *UBlock) FromBytes(argbytes []byte) (err error) {
	buf := bytes.NewBuffer(argbytes)
	// first deser the block, then the udata
	err = ub.Block.Deserialize(buf)
	if err != nil {
		return
	}
	ub.ExtraData, err = UDataFromBytes(buf.Bytes())
	return
}

// network serialization for UBlocks (regular block with udata)
// Firstjust a wire.MsgBlock with the regular serialization.
// Then there's  4 bytes is (big endian) length of the udata.
// So basically a block then udata, that's it.
// Looks like "height" doesn't get sent over this way, but maybe that's OK.
func (ub *UBlock) Deserialize(r io.Reader) (err error) {

	// first deser the block
	err = ub.Block.Deserialize(r)
	if err != nil {
		return
	}

	// fmt.Printf("deser block OK %s\n", ub.Block.Header.BlockHash().String())
	var uDataLen, bytesRead uint32
	var n int
	// read udata length
	err = binary.Read(r, binary.BigEndian, &uDataLen)
	if err != nil {
		return
	}
	// fmt.Printf("server says %d byte uDataLen\n", uDataLen)

	udataBytes := make([]byte, uDataLen)

	for bytesRead < uDataLen {
		n, err = r.Read(udataBytes[bytesRead:])
		if err != nil {
			return
		}
		bytesRead += uint32(n)
	}
	// fmt.Printf("udataBytes: %x\n", udataBytes)
	ub.ExtraData, err = UDataFromBytes(udataBytes)
	return
}

// We don't actually call serialize since from the server side we don't
// serialize, we just glom stuff together from the disk and send it over.
func (ub *UBlock) Serialize(w io.Writer) (err error) {
	var bw bytes.Buffer
	err = ub.Block.Serialize(&bw)
	if err != nil {
		return
	}

	udataBytes := ub.ExtraData.ToBytes()
	err = binary.Write(&bw, binary.BigEndian, uint32(len(udataBytes)))
	if err != nil {
		return
	}

	_, err = bw.Write(udataBytes)
	if err != nil {
		return err
	}

	payload := bw.Bytes()
	err = binary.Write(w, binary.BigEndian, uint32(len(payload)))
	if err != nil {
		return
	}
	_, err = w.Write(payload)

	return
}

// TODO use compact leafDatas in the block proofs -- probably 50%+ space savings
// Also should be default / the only serialization.  Whenever you've got the
// block proof, you've also got the block, so should always be OK to omit the
// data that's already in the block.

func UDataFromCompactBytes(b []byte) (UData, error) {
	var ud UData

	return ud, nil
}

func (ud *UData) ToCompactBytes() (b []byte) {
	return
}
