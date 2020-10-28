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
	AccProof accumulator.BatchProof
	Stxos    []LeafData
	TxoTTLs  []int32
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

// ToString turns a LeafData into a string
func (l *LeafData) ToString() (s string) {
	s = l.Outpoint.String()
	// s += fmt.Sprintf(" bh %x ", l.BlockHash)
	s += fmt.Sprintf(" h %d ", l.Height)
	s += fmt.Sprintf("cb %v ", l.Coinbase)
	s += fmt.Sprintf("amt %d ", l.Amt)
	s += fmt.Sprintf("pks %x ", l.PkScript)
	s += fmt.Sprintf("%x ", l.LeafHash())
	s += fmt.Sprintf("size %d", l.SerializeSize())
	return
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
	if len(l.PkScript) > 10000 {
		err = fmt.Errorf("pksize too long")
		return
	}
	err = binary.Write(w, binary.BigEndian, uint16(len(l.PkScript)))
	_, err = w.Write(l.PkScript)
	return
}

// SerializeSize says how big a leafdata is
func (l *LeafData) SerializeSize() int {
	// 32B blockhash, 36B outpoint, 4B h/coinbase, 8B amt, 2B pkslen, pks
	// so 82B + pks
	return 82 + len(l.PkScript)
}

func (l *LeafData) Deserialize(r io.Reader) (err error) {
	_, err = io.ReadFull(r, l.BlockHash[:])
	_, err = io.ReadFull(r, l.Outpoint.Hash[:])
	err = binary.Read(r, binary.BigEndian, &l.Outpoint.Index)
	err = binary.Read(r, binary.BigEndian, &l.Height)
	err = binary.Read(r, binary.BigEndian, &l.Amt)

	var pkSize uint16
	err = binary.Read(r, binary.BigEndian, &pkSize)
	if pkSize > 10000 {
		err = fmt.Errorf("bh %x op %s pksize %d byte too long",
			l.BlockHash, l.Outpoint.String(), pkSize)
		return
	}
	l.PkScript = make([]byte, pkSize)
	_, err = io.ReadFull(r, l.PkScript)
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

// LeafHash turns a LeafData into a LeafHash
func (l *LeafData) LeafHash() [32]byte {
	var buf bytes.Buffer
	l.Serialize(&buf)
	return sha512.Sum512_256(buf.Bytes())
}

// on disk
// aaff aaff 0000 0014 0000 0001 0000 0001 0000 0000 0000 0000 0000 0000
//  magic   |   size  |  height | numttls |   ttl0  | numTgts | ????

// ToBytes serializes UData into bytes.
// First, height, 4 bytes.
// Then, number of TTL values (4 bytes, even though we only need 2)
// Then a bunch of TTL values, (4B each) one for each txo in the associated block
// batch proof
// Bunch of LeafDatas

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

	// fmt.Printf("accproof %d bytes\n", ud.AccProof.SerializeSize())

	// write all the leafdatas
	for _, ld := range ud.Stxos {
		// fmt.Printf("writing ld %d %s\n", i, ld.ToString())
		err = ld.Serialize(w)
		if err != nil {
			return
		}
		// fmt.Printf("h %d leaf %d %s len %d\n",
		// ud.Height, i, ld.Outpoint.String(), len(ld.PkScript))
	}

	return
}

//
func (ud *UData) SerializeSize() int {
	var ldsize int
	var b bytes.Buffer

	// TODO this is slow, can remove double checking once it works reliably
	for _, l := range ud.Stxos {
		ldsize += l.SerializeSize()
		b.Reset()
		l.Serialize(&b)
		if b.Len() != l.SerializeSize() {
			fmt.Printf(" b.Len() %d, l.SerializeSize() %d\n",
				b.Len(), l.SerializeSize())
		}
	}

	b.Reset()
	ud.AccProof.Serialize(&b)
	if b.Len() != ud.AccProof.SerializeSize() {
		fmt.Printf(" b.Len() %d, AccProof.SerializeSize() %d\n",
			b.Len(), ud.AccProof.SerializeSize())
	}

	guess := 8 + (4 * len(ud.TxoTTLs)) + ud.AccProof.SerializeSize() + ldsize

	// 8B height & numTTLs, 4B per TTL, accProof size, leaf sizes
	return guess
}

func (ud *UData) Deserialize(r io.Reader) (err error) {

	err = binary.Read(r, binary.BigEndian, &ud.Height)
	if err != nil { // ^ 4B block height
		fmt.Printf("ud deser Height err %s\n", err.Error())
		return
	}
	// fmt.Printf("read height %d\n", ud.Height)

	var numTTLs int32
	err = binary.Read(r, binary.BigEndian, &numTTLs)
	if err != nil { // ^ 4B num ttls
		fmt.Printf("ud deser numTTLs err %s\n", err.Error())
		return
	}
	// fmt.Printf("read ttls %d\n", numTTLs)
	// fmt.Printf("UData deser read h %d - %d ttls ", ud.Height, numTTLs)

	ud.TxoTTLs = make([]int32, numTTLs)
	for i, _ := range ud.TxoTTLs { // write all ttls
		err = binary.Read(r, binary.BigEndian, ud.TxoTTLs[i])
		if err != nil {
			fmt.Printf("ud deser LeafTTLs[%d] err %s\n", i, err.Error())
			return
		}
		// fmt.Printf("read ttl[%d] %d\n", i, ud.TxoTTLs[i])
	}

	err = ud.AccProof.Deserialize(r)
	if err != nil { // ^ batch proof with lengths internal
		fmt.Printf("ud deser AccProof err %s\n", err.Error())
		return
	}

	// fmt.Printf("%d byte accproof, read %d targets\n",
	// ud.AccProof.SerializeSize(), len(ud.AccProof.Targets))
	// we've already gotten targets.  1 leafdata per target
	ud.Stxos = make([]LeafData, len(ud.AccProof.Targets))
	for i, _ := range ud.Stxos {
		err = ud.Stxos[i].Deserialize(r)
		if err != nil {
			err = fmt.Errorf(
				"ud deser h %d nttl %d targets %d UtxoData[%d] err %s\n",
				ud.Height, numTTLs, len(ud.AccProof.Targets), i, err.Error())
			return
		}
		// fmt.Printf("h %d leaf %d %s len %d\n",
		// ud.Height, i, ud.Stxos[i].Outpoint.String(), len(ud.Stxos[i].PkScript))

	}

	return
}

// Deserialize a UBlock.  It's just a block then udata.
func (ub *UBlock) Deserialize(r io.Reader) (err error) {
	err = ub.Block.Deserialize(r)
	if err != nil {
		return err
	}
	// fmt.Printf("deser'd block %s %d bytes\n",
	// ub.Block.Header.BlockHash().String(), ub.Block.SerializeSize())
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
