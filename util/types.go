package util

import (
	"bytes"
	"crypto/sha256"
	"crypto/sha512"
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

// UBlock is a regular block, with Udata stuck on
type UBlock struct {
	UtreexoData UData
	Block       wire.MsgBlock
}

/*
Ublock serialization
(changed with flatttl branch)

A "Ublock" is a regular bitcoin block, along with Utreexo-specific data.
The udata comes first, and the height and leafTTLs come first.

*/

type UData struct {
	Height   int32
	LeafTTLs []int32
	UtxoData []LeafData
	AccProof accumulator.BatchProof
}

// all the ttl data that comes from a block
type TtlBlock struct {
	Height  int32      // height of the block that consumed all the utxos
	Created []TxoStart // slice of
}

type TxoStart struct {
	TxBlockHeight    int32 // what block created the txo
	IndexWithinBlock int32 // index in that block where the txo is created
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
	return sha512.Sum512_256(l.ToBytes())
}

func LeafDataFromTxo(txo wire.TxOut) (LeafData, error) {
	var l LeafData

	return l, nil
}

// ToBytes serializes UData into bytes.
// First, height, 4 bytes.
// Then, number of TTL values (4 bytes, even though we only need 2)
// Then a bunch of TTL values, one for each txo in the associated block
// batch proof length (4 bytes)
// batch proof
// Bunch of LeafDatas, each prefixed with 2-byte lengths
func (ud *UData) ToBytes() []byte {
	batchBytes := ud.AccProof.ToBytes()
	buffer := new(bytes.Buffer)

	_ = binary.Write(buffer, binary.BigEndian, ud.Height)
	_ = binary.Write(buffer, binary.BigEndian, uint32(len(ud.LeafTTLs)))
	// write the TTL values
	// These start 8 bytes into each Ublock, and we need to overwrite these
	// since we don't initially know what the TTLs are
	for _, ttl := range ud.LeafTTLs {
		binary.Write(buffer, binary.BigEndian, ttl)
	}

	// write the length of the batch proof bytes
	// currently there's a redundant 4 bytes as the number of TTLs is the same
	// as the number of targets in batchBytes
	// TODO remove 4 byte redundant data here?  Annoying / not worth it?
	// it's the first 4 bytes of batchBytes so could just trim that...
	binary.Write(buffer, binary.BigEndian, uint32(len(batchBytes)))
	// write the batch proof bytes
	buffer.Write(batchBytes)
	// write the utxos data
	for _, utxo := range ud.UtxoData {
		// write the utxo data prefixed by 2 length bytes
		buffer.Write(PrefixLen16(utxo.ToBytes()))
	}
	return buffer.Bytes()
}

// UDataFromBytes deserializes into UData from bytes.
func UDataFromBytes(b []byte) (ud UData, err error) {
	// if there's no bytes, it's an empty uData
	if len(b) == 0 {
		return
	}

	if len(b) < 12 {
		err = fmt.Errorf("block proof too short %d bytes", len(b))
		return
	}

	buffer := bytes.NewBuffer(b)

	// read the height
	_ = binary.Read(buffer, binary.BigEndian, &ud.Height)
	// read number of TTLs to create the slice
	var numTTLs uint32
	_ = binary.Read(buffer, binary.BigEndian, &numTTLs)
	ud.LeafTTLs = make([]int32, numTTLs)
	for i, _ := range ud.LeafTTLs { // read each TTL, 4 bytes each
		_ = binary.Read(buffer, binary.BigEndian, &ud.LeafTTLs[i])
	}

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

	return ud, nil
}

func (ub *UBlock) FromBytes(argbytes []byte) (err error) {
	buf := bytes.NewBuffer(argbytes)
	// first deser the block, then the udata
	err = ub.Block.Deserialize(buf)
	if err != nil {
		return
	}
	ub.UtreexoData, err = UDataFromBytes(buf.Bytes())
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
	ub.UtreexoData, err = UDataFromBytes(udataBytes)
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

	udataBytes := ub.UtreexoData.ToBytes()
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
