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

// Compact deserialization gives you the dedupe skiplists for "free" so
// may as well include them here
type UBlockWithSkiplists struct {
	UBlock
	Inskip, Outskip []uint32 // really could be 16bit as no block has 65K txos
}

/*
Ublock serialization
(changed with flatttl branch)

A "Ublock" is a regular bitcoin block, along with Utreexo-specific data.
The udata comes first, and the height and leafTTLs come first.

Height: the height of the block in the blockchain
[]LeafData: UTXOs spent in this block
BatchProof: the inclustion proof for all the LeafData
TxoTTLs: for each new output created at this height, how long the utxo lasts

Note that utxos that are created & destroyed in the same block are not included
as LeafData and not proven in the BatchProof; from utreexo's perspective they
don't exist.

*/

type UData struct {
	Height   int32
	Stxos    []LeafData
	AccProof accumulator.BatchProof
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

// compact serialization flags:
// right now just 0xff for full pkscript, 0x01 for p2pkh
// TODO can add p2sh / segwit stuff later
const (
	LeafFlagFullPKScript = 0xff
	LeafFlagP2PKH        = 0x01
)

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
	if l.Height&1 == 1 {
		l.Coinbase = true
	}
	l.Height >>= 1

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

	return
}

// compact serialization for LeafData:
// don't need to send BlockHash; figure it out from height
// don't need to send outpoint, it's already in the msgBlock
// 1 byte tag for PkScript, with more if needed
// so it's just height/coinbaseness, amt, pkscript tag

// TODO can compact the amount too, same way bitcoind does

// SerializeCompact puts compact LeafData onto a writer
func (l *LeafData) SerializeCompact(w io.Writer) (err error) {
	hcb := l.Height << 1
	if l.Coinbase {
		hcb |= 1
	}

	// _, err = w.Write(l.BlockHash[:])
	// _, err = w.Write(l.Outpoint.Hash[:])
	// err = binary.Write(w, binary.BigEndian, l.Outpoint.Index)

	err = binary.Write(w, binary.BigEndian, hcb)
	err = binary.Write(w, binary.BigEndian, l.Amt)
	if len(l.PkScript) > 10000 {
		err = fmt.Errorf("pksize too long")
		return
	}
	if IsP2PKH(l.PkScript) {
		w.Write([]byte{LeafFlagP2PKH})
	} else {
		w.Write([]byte{LeafFlagFullPKScript})
		err = binary.Write(w, binary.BigEndian, uint16(len(l.PkScript)))
		_, err = w.Write(l.PkScript)
	}
	return
}

// SerializeSize says how big a leafdata is
func (l *LeafData) SerializeCompactSize() int {
	var pklen int
	if IsP2PKH(l.PkScript) {
		pklen = 1
	} else {
		pklen = 3 + len(l.PkScript) // 1 byte flag, 2 byte len, pkscript
	}
	// 4B height, 8B amount, then pkscript
	return 12 + pklen
}

// DeserializeCompact takes the bytes from SerializeCompact and rebuilds
// into a LeafDataPartial.  Note that this isn't a whole leafdata
func (l *LeafData) DeserializeCompact(r io.Reader) (flag byte, err error) {
	err = binary.Read(r, binary.BigEndian, &l.Height)
	if l.Height&1 == 1 {
		l.Coinbase = true
	}
	l.Height >>= 1

	err = binary.Read(r, binary.BigEndian, &l.Amt)

	flagSlice := make([]byte, 1) // this is dumb but only way to read 1 byte
	_, err = r.Read(flagSlice)
	flag = flagSlice[0]
	if err != nil {
		return
	}
	if flag == LeafFlagP2PKH {
		// if it's P2PKH the flag alone is enough; no PKH data is given
		return
	}

	var pkSize uint16
	err = binary.Read(r, binary.BigEndian, &pkSize)
	if pkSize > 10000 {
		err = fmt.Errorf("pksize %d byte too long", pkSize)
		return
	}
	l.PkScript = make([]byte, pkSize)
	_, err = io.ReadFull(r, l.PkScript)

	return
}

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

// Serialize but use compact encoding for leafData
func (ud *UData) SerializeCompact(w io.Writer) (err error) {
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
		err = ld.SerializeCompact(w)
		if err != nil {
			return
		}
		// fmt.Printf("h %d leaf %d %s len %d\n",
		// ud.Height, i, ld.Outpoint.String(), len(ld.PkScript))
	}

	return
}

// gives the size of the serialized udata without actually serializing it
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

// gives the size of the serialized udata with compact leaf data
func (ud *UData) SerializeCompactSize() int {
	var ldsize int
	var b bytes.Buffer

	// TODO this is slow, can remove double checking once it works reliably
	for _, l := range ud.Stxos {
		ldsize += l.SerializeCompactSize()
		b.Reset()
		l.SerializeCompact(&b)
		if b.Len() != l.SerializeCompactSize() {
			fmt.Printf(" b.Len() %d, l.SerializeCompactSize() %d\n",
				b.Len(), l.SerializeCompactSize())
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

	var numTTLs uint32
	err = binary.Read(r, binary.BigEndian, &numTTLs)
	if err != nil { // ^ 4B num ttls
		fmt.Printf("ud deser numTTLs err %s\n", err.Error())
		return
	}
	// fmt.Printf("read ttls %d\n", numTTLs)
	// fmt.Printf("UData deser read h %d - %d ttls ", ud.Height, numTTLs)

	ud.TxoTTLs = make([]int32, numTTLs)
	for i, _ := range ud.TxoTTLs { // write all ttls
		err = binary.Read(r, binary.BigEndian, &ud.TxoTTLs[i])
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

// Gives a partially filled in UData from compact serialization
// Also gives the "flags" for the leaf data.  Combine those with
// The data from the regular block to recreate the full leaf data
func (ud *UData) DeserializeCompact(r io.Reader) (flags []byte, err error) {
	err = binary.Read(r, binary.BigEndian, &ud.Height)
	if err != nil { // ^ 4B block height
		fmt.Printf("ud deser Height err %s\n", err.Error())
		return
	}
	// fmt.Printf("read height %d\n", ud.Height)

	var numTTLs uint32
	err = binary.Read(r, binary.BigEndian, &numTTLs)
	if err != nil { // ^ 4B num ttls
		fmt.Printf("ud deser numTTLs err %s\n", err.Error())
		return
	}
	// fmt.Printf("read ttls %d\n", numTTLs)
	// fmt.Printf("UData deser read h %d - %d ttls ", ud.Height, numTTLs)

	ud.TxoTTLs = make([]int32, numTTLs)
	for i, _ := range ud.TxoTTLs { // write all ttls
		err = binary.Read(r, binary.BigEndian, &ud.TxoTTLs[i])
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
	flags = make([]byte, len(ud.AccProof.Targets))
	var flag byte
	for i, _ := range ud.Stxos {
		flag, err = ud.Stxos[i].DeserializeCompact(r)
		if err != nil {
			err = fmt.Errorf(
				"ud deser h %d nttl %d targets %d UtxoData[%d] err %s\n",
				ud.Height, numTTLs, len(ud.AccProof.Targets), i, err.Error())
			return
		}
		flags[i] = flag
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

// We don't actually call serialize since from the server side we don't
// serialize, we just glom stuff together from the disk and send it over.
func (ub *UBlock) SerializeCompact(w io.Writer) (err error) {
	err = ub.Block.Serialize(w)
	if err != nil {
		return
	}
	err = ub.UtreexoData.SerializeCompact(w)
	return
}

// SerializeSize: how big is it, in bytes.
func (ub *UBlock) SerializeCompactSize() int {
	return ub.Block.SerializeSize() + ub.UtreexoData.SerializeCompactSize()
}

// Deserialize a compact UBlock.  More complex in that the leafdata gets
// rebuilt from the block data.  Note that this leaves the blockhash
// empty in the leaf data, so that needs to be filled in by lookup up
// the headers (block height is provided)
// The 2 things to rebuild here are outpoint and pkscript
// Also we need a skiplist here as 0-duration UTXOs don't get proofs, so
// they show up in the ub.Block but not in ub.UtreexoData.
// Return the skiplist so you don't have to calculate it twice.
func (ub *UBlockWithSkiplists) DeserializeCompact(r io.Reader) (err error) {
	err = ub.Block.Deserialize(r)
	if err != nil {
		return err
	}
	// get the skiplists from the block & save them in the ubwsls
	ub.Inskip, ub.Outskip = DedupeBlock(&ub.Block)

	// fmt.Printf("deser'd block %s %d bytes\n",
	// ub.Block.Header.BlockHash().String(), ub.Block.SerializeSize())
	flags, err := ub.UtreexoData.DeserializeCompact(r)

	// ensure leaf data & block inputs size match up
	if len(flags) != len(ub.UtreexoData.Stxos) {
		err = fmt.Errorf("%d flags but %d leaf data",
			len(flags), len(ub.UtreexoData.Stxos))
	}
	// make sure the number of targets in the proof side matches the
	// number of inputs in the block
	proofsRemaining := len(flags)
	for i, tx := range ub.Block.Transactions {
		if i == 0 {
			continue
		}
		proofsRemaining -= len(tx.TxIn)
	}
	// if it doesn't match, fail
	if proofsRemaining != 0 {
		err = fmt.Errorf("%d txos proven but %d inputs in block",
			len(flags), len(flags)-proofsRemaining)
	}

	// blockToDelOPs()
	// we know the leaf data & inputs match up, at least in number, so
	// rebuild the leaf data.  It could be wrong but we'll find out later
	// if the hashes / proofs don't match.
	inputInBlock := 0
	skippos := 0
	skiplen := len(ub.Inskip)
	// fmt.Printf("%d h %d txs %d targets inskip %v\n",
	// ub.UtreexoData.Height, len(ub.Block.Transactions),
	// len(ub.UtreexoData.Stxos), ub.Inskip)
	for i, tx := range ub.Block.Transactions {
		if i == 0 {
			continue // skip coinbase, not counted in Stxos
		}
		// loop through inputs
		for _, in := range tx.TxIn {
			// skip if on skiplist
			if skippos < skiplen && ub.Inskip[skippos] == uint32(inputInBlock) {
				skippos++
				inputInBlock++
				continue
			}

			// rebuild leaf data from this txin data (OP and PkScript)
			// copy outpoint from block into leaf
			ub.UtreexoData.Stxos[inputInBlock].Outpoint = in.PreviousOutPoint
			// rebuild pkscript based on flag

			// so far only P2PKH are omitted / recovered
			if flags[inputInBlock] == LeafFlagP2PKH {
				// get pubkey from sigscript
				ub.UtreexoData.Stxos[inputInBlock].PkScript, err =
					RecoverPkScriptP2PKH(in.SignatureScript)
				if err != nil {
					return
				}
			}
			inputInBlock++
		}
	}

	return nil
}
