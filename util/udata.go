package util

import (
	"fmt"

	"github.com/mit-dci/utreexo/accumulator"
)

// Verify checks the consistency of uData: that the utxos are proven in the
// batchproof
func (ud *UData) Verify(nl uint64, h uint8) bool {

	// this is really ugly and basically copies the whole thing to avoid
	// destroying it while verifying...

	presort := make([]uint64, len(ud.AccProof.Targets))
	hashstash := make([]accumulator.Hash, len(ud.AccProof.Proof))
	copy(presort, ud.AccProof.Targets)
	copy(hashstash, ud.AccProof.Proof)
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
	ud.AccProof.Proof = hashstash
	return true
}
