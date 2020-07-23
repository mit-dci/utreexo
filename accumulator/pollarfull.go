package accumulator

import (
	"fmt"
)

// read is just like forestData read but for pollard
func (p *Pollard) read(pos uint64) Hash {
	n, _, _, err := p.grabPos(pos)
	if err != nil {
		fmt.Printf("read err %s pos %d\n", err.Error(), pos)
		return empty
	}
	if n == nil {
		return empty
	}
	return n.data
}

// NewFullPollard gives you a Pollard with an activated
func NewFullPollard() Pollard {
	var p Pollard
	p.positionMap = make(map[MiniHash]uint64)
	return p
}

// PosMapSanity is costly / slow: check that everything in posMap is correct
func (p *Pollard) PosMapSanity() error {
	for i := uint64(0); i < p.numLeaves; i++ {
		if p.positionMap[p.read(i).Mini()] != i {
			return fmt.Errorf("positionMap error: map says %x @%d but it's @%d",
				p.read(i).Prefix(), p.positionMap[p.read(i).Mini()], i)
		}
	}
	return nil
}

// TODO make interface to reduce code dupe

// ProveBatch but for pollard.
// Now getting really obvious that forest and pollard should both satisfy some
// kind of utreexo-like interface.  And maybe forest shouldn't be called forest.
// Anyway do that after this.
func (p *Pollard) ProveBatch(hs []Hash) (BatchProof, error) {
	var bp BatchProof
	// skip everything if empty (should this be an error?
	if len(hs) == 0 {
		return bp, nil
	}
	if p.numLeaves < 2 {
		return bp, nil
	}

	// for h, p := range f.positionMap {
	// 	fmt.Printf("%x@%d ", h[:4], p)
	// }

	// first get all the leaf positions
	// there shouldn't be any duplicates in hs, but if there are I guess
	// it's not an error.
	bp.Targets = make([]uint64, len(hs))

	for i, wanted := range hs {

		pos, ok := p.positionMap[wanted.Mini()]
		if !ok {
			fmt.Print(p.ToString())
			return bp, fmt.Errorf("hash %x not found", wanted)
		}

		// should never happen
		if pos > p.numLeaves {
			for m, p := range p.positionMap {
				fmt.Printf("%x @%d\t", m[:4], p)
			}
			return bp, fmt.Errorf(
				"ProveBlock: got leaf position %d but only %d leaves exist",
				pos, p.numLeaves)
		}
		bp.Targets[i] = pos
	}
	// targets need to be sorted because the proof hashes are sorted
	// NOTE that this is a big deal -- we lose in-block positional information
	// because of this sorting.  Does that hurt locality or performance?  My
	// guess is no, but that's untested.
	sortUint64s(bp.Targets)

	// TODO feels like you could do all this with just slices and no maps...
	// that would be better
	// proofTree is the partially populated tree of everything needed for the
	// proofs
	proofTree := make(map[uint64]Hash)

	// go through each target and add a proof for it up to the intersection
	for _, pos := range bp.Targets {
		// add hash for the deletion itself and its sibling
		// if they already exist, skip the whole thing
		_, alreadyThere := proofTree[pos]
		if alreadyThere {
			//			fmt.Printf("%d omit already there\n", pos)
			continue
		}
		// TODO change this for the real thing; no need to prove 0-tree root.
		// but we still need to verify it and tag it as a target.
		if pos == p.numLeaves-1 && pos&1 == 0 {
			proofTree[pos] = p.read(pos)
			// fmt.Printf("%d add as root\n", pos)
			continue
		}

		// always put in both siblings when on the bottom row
		// this can be out of order but it will be sorted later
		proofTree[pos] = p.read(pos)
		proofTree[pos^1] = p.read(pos ^ 1)
		// fmt.Printf("added leaves %d, %d\n", pos, pos^1)

		treeRoot := detectSubTreeRows(pos, p.numLeaves, p.rows())
		pos = parent(pos, p.rows())
		// go bottom to top and add siblings into the partial tree
		// start at row 1 though; we always populate the bottom leaf and sibling
		// This either gets to the top, or intersects before that and deletes
		// something
		for r := uint8(1); r < treeRoot; r++ {
			// check if the sibling is already there, in which case we're done
			// also check if the parent itself is there, in which case we delete it!
			// I think this with the early ignore at the bottom make it optimal
			_, selfThere := proofTree[pos]
			_, sibThere := proofTree[pos^1]
			if sibThere {
				// sibling position already exists in partial tree; done
				// with this branch

				// TODO seems that this never happens and can be removed
				panic("this never happens...?")
			}
			if selfThere {
				// self position already there; remove as children are known
				//				fmt.Printf("remove proof from pos %d\n", pos)

				delete(proofTree, pos)
				delete(proofTree, pos^1) // right? can delete both..?
				break
			}
			// fmt.Printf("add proof from pos %d\n", pos^1)
			proofTree[pos^1] = p.read(pos ^ 1)
			pos = parent(pos, p.rows())
		}
	}

	var nodeSlice []node

	// run through partial tree to turn it into a slice
	for pos, hash := range proofTree {
		nodeSlice = append(nodeSlice, node{pos, hash})
	}
	// fmt.Printf("made nodeSlice %d nodes\n", len(nodeSlice))

	// sort the slice of nodes (even though we only want the hashes)
	sortNodeSlice(nodeSlice)
	// copy the sorted / in-order hashes into a hash slice
	bp.Proof = make([]Hash, len(nodeSlice))

	for i, n := range nodeSlice {
		bp.Proof[i] = n.Val
	}
	if verbose {
		fmt.Printf("blockproof targets: %v\n", bp.Targets)
	}

	return bp, nil
}
