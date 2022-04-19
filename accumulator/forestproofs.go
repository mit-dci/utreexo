package accumulator

import (
	"fmt"
	"time"
)

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
