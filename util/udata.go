package util

import "fmt"

func (ud *UData) Verify(nl uint64, h uint8) bool {

	mp, err := ud.AccProof.Reconstruct(nl, h)
	if err != nil {
		fmt.Printf(" Reconstruct failed %s\n")
		return false
	}

	// make sure the udata is consistent, with the same number of leafDatas
	// as targets in the accumulator batch proof
	if len(ud.AccProof.Targets) != len(ud.UtxoData) {
		fmt.Printf("Verify failed: %d targets but %d leafdatas\n",
			len(ud.AccProof.Targets), len(ud.UtxoData))
	}

	// fmt.Printf("%d proofs ", len(ud.AccProof.Proof))
	// for i, h := range ud.AccProof.Proof {
	// 	fmt.Printf("%d %x\t", i, h[:4])
	// }

	for i, pos := range ud.AccProof.Targets {
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
	return true
}
