package accumulator

import (
	"fmt"
	"time"
)

// Proof :
type Proof struct {
	Position uint64 // where at the bottom of the tree it sits
	Payload  Hash   // hash of the thing itself (what's getting proved)
	Siblings []Hash // slice of siblings up to a root
}

// Verify checks an inclusion proof.
// returns false on any errors
func (f *Forest) Verify(p Proof) bool {

	n := p.Payload
	//	fmt.Printf("check position %d %04x inclusion\n", p.Position, n[:4])

	subTreeRows := detectSubTreeRows(p.Position, f.numLeaves, f.rows)
	// there should be as many siblings as the rows of the sub-tree
	// (0 rows means there are no siblings; there is no proof)
	if uint8(len(p.Siblings)) != subTreeRows {
		fmt.Printf("proof wrong size, expect %d got %d\n",
			subTreeRows, len(p.Siblings))
		return false
	}
	//	fmt.Printf("verify %04x\n", n[:4])
	for h, sib := range p.Siblings {
		// fmt.Printf("%04x ", sib[:4])
		// detect current row parity
		if 1<<uint(h)&p.Position == 0 {
			//			fmt.Printf("compute %04x %04x -> ", n[:4], sib[:4])
			n = parentHash(n, sib)
			//			fmt.Printf("%04x\n", n[:4])
		} else {
			//			fmt.Printf("compute %04x %04x -> ", sib[:4], n[:4])
			n = parentHash(sib, n)
			//			fmt.Printf("%04x\n", n[:4])
		}
	}

	subTreeRootPos := parentMany(p.Position, subTreeRows, f.rows)

	if subTreeRootPos >= f.data.size() {
		fmt.Printf("ERROR don't have root at %d\n", subTreeRootPos)
		return false
	}
	subRoot := f.data.read(subTreeRootPos)

	if n != subRoot {
		fmt.Printf("got %04x subroot %04x\n", n[:4], subRoot[:4])
	}
	return n == subRoot
}

// VerifyMany is like verify but more.
func (f *Forest) VerifyMany(ps []Proof) bool {
	for _, p := range ps {
		if !f.Verify(p) {
			return false
		}
	}
	return true
}

// ProveBatch gets proofs (in the form of a node slice) for a bunch of leaves
// The ordering of Targets is the same as the ordering of hashes given as
// argument.
//
// NOTE: The order in which the hashes are given matter when verifying
// (aka permutation matters).
func (f *Forest) ProveBatch(hs []Hash) (BatchProof, error) {
	starttime := time.Now()
	var bp BatchProof
	// skip everything if empty (should this be an error?
	if len(hs) == 0 {
		return bp, nil
	}
	// When there is only 1 leaf in the entire forest, the leaf is the proof.
	// When there are no leaves, there's nothing to prove.
	if f.numLeaves <= 1 {
		return bp, nil
	}

	// first get all the leaf positions
	// there shouldn't be any duplicates in hs, but if there are I guess
	// it's not an error.
	bp.Targets = make([]uint64, len(hs))

	for i, wanted := range hs {
		pos, ok := f.positionMap[wanted.Mini()]
		if !ok {
			fmt.Print(f.ToString())
			return bp, fmt.Errorf("hash %x not found", wanted)
		}

		// should never happen
		if pos > f.numLeaves {
			for m, p := range f.positionMap {
				fmt.Printf("%x @%d\t", m[:4], p)
			}
			return bp, fmt.Errorf(
				"ProveBatch: got leaf position %d but only %d leaves exist",
				pos, f.numLeaves)
		}
		bp.Targets[i] = pos
	}
	// targets need to be sorted because the proof hashes are sorted
	// NOTE that this is a big deal -- we lose in-block positional information
	// because of this sorting.  Does that hurt locality or performance?  My
	// guess is no, but that's untested.
	sortedTargets := make([]uint64, len(bp.Targets))
	copy(sortedTargets, bp.Targets)
	sortUint64s(sortedTargets)

	proofPositions := NewPositionList()
	defer proofPositions.Free()

	// Get the positions of all the hashes that are needed to prove the targets
	ProofPositions(sortedTargets, f.numLeaves, f.rows, &proofPositions.list)

	bp.Proof = make([]Hash, len(proofPositions.list))
	for i, proofPos := range proofPositions.list {
		bp.Proof[i] = f.data.read(proofPos)
	}

	if verbose {
		fmt.Printf("blockproof targets: %v\n", bp.Targets)
	}

	donetime := time.Now()
	f.timeInProve += donetime.Sub(starttime)
	return bp, nil
}

// VerifyBatchProof is just a wrapper around verifyBatchProof
func (f *Forest) VerifyBatchProof(toProve []Hash, bp BatchProof) error {
	_, _, err := verifyBatchProof(toProve, bp, f.GetRoots(), f.numLeaves, nil)
	return err
}
