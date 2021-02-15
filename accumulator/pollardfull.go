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

	positionList := NewPositionList()
	defer positionList.Free()
	ProofPositions(bp.Targets, p.numLeaves, p.rows(), &positionList.list)
	targetsAndProof := mergeSortedSlices(positionList.list, bp.Targets)
	bp.Proof = make([]Hash, len(targetsAndProof))
	for i, proofPos := range targetsAndProof {
		bp.Proof[i] = p.read(proofPos)
	}

	if verbose {
		fmt.Printf("blockproof targets: %v\n", bp.Targets)
	}

	return bp, nil
}
