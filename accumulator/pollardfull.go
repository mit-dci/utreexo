package accumulator

import (
	"encoding/hex"
	"fmt"
)

func (p *polNode) calculatePosition(numLeaves uint64, roots []*polNode) uint64 {
	// Tells whether to follow the left child or the right child when going
	// down the tree. 0 means left, 1 means right.
	leftRightIndicator := uint64(0)

	polNode := p

	//auntHash := "nil"
	//if polNode.aunt != nil {
	//	auntHash = hex.EncodeToString(polNode.aunt.data[:])
	//}

	//fmt.Printf("starting with polnode with hash %s, and aunthash %s\n",
	//	hex.EncodeToString(polNode.data[:]), auntHash)

	rowsToTop := 0
	for polNode.aunt != nil {
		//fmt.Printf("aunt %s\n", hex.EncodeToString(polNode.aunt.data[:]))
		//fmt.Printf("aunt.niece[0] %s\n", hex.EncodeToString(polNode.aunt.niece[0].data[:]))
		//if polNode.aunt.niece[0] == polNode {
		if polNode.aunt.leftNiece == polNode {
			leftRightIndicator <<= 1
			//fmt.Println("left ", strconv.FormatUint(leftRightIndicator, 2))
		} else {
			leftRightIndicator <<= 1
			leftRightIndicator |= 1

			//fmt.Println("right", strconv.FormatUint(leftRightIndicator, 2))
		}

		polNode = polNode.aunt

		// Ugly but need to flip the bit that we set in this loop as the roots
		// don't point their children instead of their nieces.
		if rowsToTop == 0 {
			leftRightIndicator ^= 1
		}
		rowsToTop++
	}
	//fmt.Printf("leftRightIndicator %s, leftRightIndicator^1 %s\n", strconv.FormatUint(leftRightIndicator, 2), strconv.FormatUint(leftRightIndicator^1, 2))

	forestRows := treeRows(numLeaves)

	// Calculate which row the root is on.
	rootRow := 0
	// Start from the lowest root.
	rootIdx := len(roots) - 1
	for h := 0; h <= int(forestRows); h++ {
		//fmt.Println("h", h)

		// Because every root represents a perfect tree of every leaf
		// we ever added, each root position will be a power of 2.
		//
		// Go through the bits of numLeaves. Every bit that is on
		// represents a root.
		if (numLeaves>>h)&1 == 1 {
			// If we found the root, save the row to rootRow
			// and return.
			if roots[rootIdx] == polNode {
				rootRow = h
				break
			}

			// Check the next higher root.
			rootIdx--
		}
	}

	//fmt.Println("leftRightIndicator: ", strconv.FormatUint(leftRightIndicator, 2))
	//fmt.Println("rows: ", rows)
	//fmt.Println("rootrow: ", rootRow)
	//fmt.Println("numLeaves: ", numLeaves)
	//fmt.Println("polNode: ", hex.EncodeToString(polNode.data[:]))
	//fmt.Println("rootPos: ", rootPosition(numLeaves, uint8(rootRow), treeRows(numLeaves)))

	// Start from the root and work our way down the position that we want.
	retPos := rootPosition(numLeaves, uint8(rootRow), forestRows)

	for i := 0; i < rowsToTop; i++ {
		isRight := uint64(1) << i
		if leftRightIndicator&isRight == isRight {
			//fmt.Printf("current pos %d, going to %d\n",
			//	retPos, child(retPos, treeRows(numLeaves))^1)

			// Grab the sibling since the pollard nodes point to their
			// niece. My sibling's nieces are my children.
			retPos = sibling(rightChild(retPos, forestRows))
		} else {
			//fmt.Printf("current pos %d, going to %d\n",
			//	retPos, rightChild(retPos, treeRows(numLeaves))^1)

			// Grab the sibling since the pollard nodes point to their
			// niece. My sibling's nieces are my children.
			retPos = sibling(child(retPos, forestRows))
		}
	}

	return retPos
}

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
//
// NOTE: The order in which the hashes are given matter when verifying
// (aka permutation matters).
func (p *Pollard) ProveBatch(hs []Hash) (BatchProof, error) {
	var bp BatchProof
	// skip everything if empty (should this be an error?
	if len(hs) == 0 {
		return bp, nil
	}
	if p.numLeaves < 2 {
		return bp, nil
	}

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
	sortedTargets := make([]uint64, len(bp.Targets))
	copy(sortedTargets, bp.Targets)
	sortUint64s(sortedTargets)

	proofPositions := NewPositionList()
	defer proofPositions.Free()

	// Get the positions of all the hashes that are needed to prove the targets
	ProofPositions(sortedTargets, p.numLeaves, p.rows(), &proofPositions.list)

	bp.Proof = make([]Hash, len(proofPositions.list))
	for i, proofPos := range proofPositions.list {
		bp.Proof[i] = p.read(proofPos)
	}

	if verbose {
		fmt.Printf("blockproof targets: %v\n", bp.Targets)
	}

	return bp, nil
}

func (p *Pollard) ProveBatchSwapless(hashes []Hash) (BatchProof, error) {
	var bp BatchProof
	// skip everything if empty (should this be an error?
	if len(hashes) == 0 {
		return bp, nil
	}
	if p.numLeaves < 2 {
		return bp, nil
	}

	// first get all the leaf positions
	// there shouldn't be any duplicates in hs, but if there are I guess
	// it's not an error.
	bp.Targets = make([]uint64, len(hashes))

	for i, wanted := range hashes {
		node, ok := p.NodeMap[wanted.Mini()]
		if !ok {
			fmt.Print(p.ToString())
			return bp, fmt.Errorf("ProveBatchSwapless hash %x not found", wanted)
		}

		pos := node.calculatePosition(p.numLeaves, p.roots)

		n, _, _, err := p.readPos(pos)
		if err != nil {
			return bp, err
		}
		if n == nil {
			return bp, fmt.Errorf("failed to read %d, wanted %s",
				pos, hex.EncodeToString(wanted[:]))
		}
		if n.data != wanted {
			return bp, fmt.Errorf("pos %d: wanted %s, got %s",
				pos, hex.EncodeToString(wanted[:]), hex.EncodeToString(n.data[:]))
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
	ProofPositions(sortedTargets, p.numLeaves, p.rows(), &proofPositions.list)

	bp.Proof = make([]Hash, len(proofPositions.list))
	for i, proofPos := range proofPositions.list {
		bp.Proof[i] = p.read(proofPos)
	}

	if verbose {
		fmt.Printf("blockproof targets: %v\n", bp.Targets)
	}

	return bp, nil
}
