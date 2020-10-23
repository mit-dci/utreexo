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
	TxoTTLs  []int32
	Stxos    []LeafData
	AccProof accumulator.BatchProof
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

// Serialize puts LeafData onto a writer
func (l *LeafData) Serialize(w io.Writer) (err error) {
	hcb := l.Height << 1
	if l.Coinbase {
		hcb |= 1
	}

	_, err = w.Write(l.BlockHash[:])
	_, err = w.Write(l.Outpoint.Hash[:])
	err = binary.Write(w, binary.BigEndian, l.Outpoint.Index)
	err = binary.Write(w, binary.BigEndian, hcb)
	err = binary.Write(w, binary.BigEndian, l.Amt)
	err = binary.Write(w, binary.BigEndian, uint32(len(l.PkScript)))
	_, err = w.Write(l.PkScript)

	// lazy but I guess if err is ever non-nil it'll keep doing that
	return
}

// SerializeSize says how big a leafdata is
func (l *LeafData) SerializeSize() int {
	// 32B blockhash, 36B outpoint, 4B h/coinbase, 8B amt, 4B pkslen, pks
	// so 84B + pks
	return 84 + len(l.PkScript)
}

func (l *LeafData) Deserialize(r io.Reader) (err error) {
	_, err = r.Read(l.BlockHash[:])
	_, err = r.Read(l.Outpoint.Hash[:])
	err = binary.Read(r, binary.BigEndian, &l.Outpoint.Index)
	err = binary.Read(r, binary.BigEndian, &l.Height)
	err = binary.Read(r, binary.BigEndian, &l.Amt)

	var pkSize uint32
	err = binary.Read(r, binary.BigEndian, &pkSize)
	l.PkScript = make([]byte, pkSize)
	_, err = r.Read(l.PkScript)

	if l.Height&1 == 1 {
		l.Coinbase = true
	}
	l.Height >>= 1
	return
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

func (ud *UData) ToBytes() []byte {
	batchBytes := ud.AccProof.ToBytes()
	buffer := new(bytes.Buffer)

	_ = binary.Write(buffer, binary.BigEndian, ud.Height)
	_ = binary.Write(buffer, binary.BigEndian, uint32(len(ud.TxoTTLs)))
	// write the TTL values
	// These start 8 bytes into each Ublock, and we need to overwrite these
	// since we don't initially know what the TTLs are
	for _, ttl := range ud.TxoTTLs {
		binary.Write(buffer, binary.BigEndian, ttl)
	}

	// write the length of the batch proof bytes
	binary.Write(buffer, binary.BigEndian, uint32(len(batchBytes)))
	// write the batch proof bytes
	buffer.Write(batchBytes)
	// write the utxos data
	for _, utxo := range ud.Stxos {
		// write the utxo data prefixed by 2 length bytes
		buffer.Write(PrefixLen16(utxo.ToBytes()))
	}
	return buffer.Bytes()
}

// on disk
// aaff aaff 0000 0014 0000 0001 0000 0001 0000 0000 0000 0000 0000 0000
//  magic   |   size  |  height | numttls |   ttl0  | numTgts | ????

// ToBytes serializes UData into bytes.
// First, height, 4 bytes.
// Then, number of TTL values (4 bytes, even though we only need 2)
// Then a bunch of TTL values, one for each txo in the associated block
// batch proof length (4 bytes)
// batch proof
// Bunch of LeafDatas, each prefixed with 2-byte lengths

func (ud *UData) Serialize(w io.Writer) (err error) {
	err = binary.Write(w, binary.BigEndian, ud.Height)
	if err != nil { // ^ 4B block height
		return
	}
	err = binary.Write(w, binary.BigEndian, uint32(len(ud.TxoTTLs)))
	if err != nil { // ^ 4B num ttls
		return
	}
	for _, ttlval := range ud.TxoTTLs { // write all ttls
		err = binary.Write(w, binary.BigEndian, ttlval)
		if err != nil {
			return
		}
	}

	err = ud.AccProof.Serialize(w)
	if err != nil { // ^ batch proof with lengths internal
		return
	}
	fmt.Printf("accproof %d bytes\n", ud.AccProof.SerializeSize())

	// write all the leafdatas
	for _, ld := range ud.Stxos {
		err = ld.Serialize(w)
		if err != nil {
			return
		}
	}

	return
}

//
func (ud *UData) SerializeSize() int {
	var ldsize int
	for _, l := range ud.Stxos {
		ldsize += l.SerializeSize()
	}
	// 8B height & numTTLs, 4B per TTL, accProof size, leaf sizes
	return 8 + (4 * len(ud.TxoTTLs)) + ud.AccProof.SerializeSize() + ldsize
}

func (ud *UData) Deserialize(r io.Reader) (err error) {

	err = binary.Read(r, binary.BigEndian, &ud.Height)
	if err != nil { // ^ 4B block height
		fmt.Printf("ud deser Height err %s\n", err.Error())
		return
	}

	var numTTLs int32
	err = binary.Read(r, binary.BigEndian, &numTTLs)
	if err != nil { // ^ 4B num ttls
		fmt.Printf("ud deser numTTLs err %s\n", err.Error())
		return
	}

	fmt.Printf("UData deser read h %d - %d ttls ", ud.Height, numTTLs)

	ud.TxoTTLs = make([]int32, numTTLs)
	for i, _ := range ud.TxoTTLs { // write all ttls
		err = binary.Read(r, binary.BigEndian, ud.TxoTTLs[i])
		if err != nil {
			fmt.Printf("ud deser LeafTTLs[%d] err %s\n", i, err.Error())
			return
		}
	}

	err = ud.AccProof.Deserialize(r)
	if err != nil { // ^ batch proof with lengths internal
		fmt.Printf("ud deser AccProof err %s\n", err.Error())
		return
	}
	fmt.Printf("%d leaves\n", len(ud.AccProof.Targets))
	// we've already gotten targets.  1 leafdata per target
	ud.Stxos = make([]LeafData, len(ud.AccProof.Targets))
	for i, _ := range ud.Stxos {
		err = ud.Stxos[i].Deserialize(r)
		if err != nil {
			fmt.Printf("ud deser UtxoData[%d] err %s\n", i, err.Error())
			return
		}
	}

	return
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
	ud.TxoTTLs = make([]int32, numTTLs)
	for i, _ := range ud.TxoTTLs { // read each TTL, 4 bytes each
		_ = binary.Read(buffer, binary.BigEndian, &ud.TxoTTLs[i])
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
	ud.Stxos = make([]LeafData, len(ud.AccProof.Targets))
	for i, _ := range ud.Stxos {
		dataSize := binary.BigEndian.Uint16(buffer.Next(2))
		dataBytes := buffer.Next(int(dataSize))
		ud.Stxos[i], err = LeafDataFromBytes(dataBytes)
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

// Deserialize a UBlock.  It's just a block then udata.
func (ub *UBlock) Deserialize(r io.Reader) (err error) {
	err = ub.Block.Deserialize(r)
	if err != nil {
		return err
	}
	fmt.Printf("got a block %s\n", ub.Block.Header.BlockHash().String())
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
