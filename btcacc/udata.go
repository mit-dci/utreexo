package btcacc

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/mit-dci/utreexo/accumulator"
)

type UData struct {
	Height   int32
	AccProof accumulator.BatchProof
	Stxos    []LeafData
	TxoTTLs  []int32
}

// Verify checks the consistency of uData: that the utxos are proven in the
// batchproof
func (ud *UData) ProofSanity(nl uint64, h uint8) bool {
	// this is really ugly and basically copies the whole thing to avoid
	// destroying it while verifying...
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
			fmt.Printf("Verify failed: txhash %x index %d pos %d leafdata %x in proof %x\n",
				ud.Stxos[i].TxHash, ud.Stxos[i].Index, pos,
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

// GenUData creates a block proof, calling forest.ProveBatch with the leaf indexes
// to get a batched inclusion proof from the accumulator. It then adds on the leaf data,
// to create a block proof which both proves inclusion and gives all utxo data
// needed for transaction verification.
func GenUData(delLeaves []LeafData, forest *accumulator.Forest, height int32) (
	ud UData, err error) {

	ud.Height = height
	ud.Stxos = delLeaves
	// make slice of hashes from leafdata
	delHashes := make([]accumulator.Hash, len(ud.Stxos))
	for i, _ := range ud.Stxos {
		delHashes[i] = ud.Stxos[i].LeafHash()
	}
	// generate block proof. Errors if the tx cannot be proven
	// Should never error out with genproofs as it takes
	// blk*.dat files which have already been vetted by Bitcoin Core
	ud.AccProof, err = forest.ProveBatch(delHashes)
	if err != nil {
		err = fmt.Errorf("genUData failed at block %d %s %s",
			height, forest.Stats(), err.Error())
		return
	}

	if len(ud.AccProof.Targets) != len(delLeaves) {
		err = fmt.Errorf("genUData %d targets but %d leafData",
			len(ud.AccProof.Targets), len(delLeaves))
		return
	}

	// fmt.Printf(ud.AccProof.ToString())
	return
}
