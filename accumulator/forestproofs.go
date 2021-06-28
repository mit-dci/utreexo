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

// Prove :
func (f *Forest) Prove(wanted Hash) (Proof, error) {
	starttime := time.Now()

	var pr Proof
	var empty [32]byte
	// first look up where the hash is
	pos, ok := f.positionMap[wanted.Mini()]
	if !ok {
		return pr, fmt.Errorf("hash %x not found", wanted)
	}

	// should never happen
	if pos > f.numLeaves {
		return pr, fmt.Errorf("prove: got leaf position %d but only %d leaves exist",
			pos, f.numLeaves)
	}

	// build empty proof branch slice of siblings
	// not full rows -- need to figure out which subtree it's in!
	pr.Siblings = make([]Hash, detectSubTreeRows(pos, f.numLeaves, f.rows))
	pr.Payload = f.data.read(pos)
	if pr.Payload != wanted {
		return pr, fmt.Errorf(
			"prove: forest and position map conflict. want %x got %x at pos %d",
			wanted[:4], pr.Payload[:4], pos)
	}
	pr.Position = pos
	//	fmt.Printf("nl %d proof for %d len %d\n", f.numLeaves, pos, len(pr.Siblings))
	//	fmt.Printf("\tprove pos %d %x:\n", pos, pr.Payload[:4])
	// go up and populate the siblings
	for h, _ := range pr.Siblings {

		pr.Siblings[h] = f.data.read(pos ^ 1)
		if pr.Siblings[h] == empty {
			fmt.Print(f.ToString())
			return pr, fmt.Errorf(
				"prove: got empty hash proving leaf %d row %d pos %d nl %d",
				pr.Position, h, pos^1, f.numLeaves)
		}
		//		fmt.Printf("sibling %d: pos %d %x\n", h, pos^1, pr.Siblings[h][:4])
		pos = parent(pos, f.rows)

	}

	donetime := time.Now()
	f.timeInProve += donetime.Sub(starttime)
	return pr, nil
}

// ProveMany :
func (f *Forest) ProveMany(hs []Hash) ([]Proof, error) {
	var err error
	proofs := make([]Proof, len(hs))
	for i, h := range hs {
		proofs[i], err = f.Prove(h)
		if err != nil {
			return proofs, err
		}
	}
	return proofs, err
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
	if f.data.size() < 2 {
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
func (f *Forest) VerifyBatchProof(toProve []Hash, bp BatchProof) bool {
	ok, _, _ := verifyBatchProof(toProve, bp, f.getRoots(), f.numLeaves, nil)
	return ok
}
