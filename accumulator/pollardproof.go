package accumulator

import (
	"fmt"
)

// IngestBatchProof populates the Pollard with all needed data to delete the
// targets in the block proof
func (p *Pollard) IngestBatchProof(bp BatchProof) error {
	var empty Hash
	// TODO so many things to change
	// Verify the proofs that was sent and returns a map of proofs to locations
	ok, proofMap := p.verifyBatchProof(
		bp, p.rootHashesReverse(), p.numLeaves, p.rows())
	if !ok {
		return fmt.Errorf("block proof mismatch")
	}
	// go through each target and populate pollard
	for _, target := range bp.Targets {
		tNum, branchLen, bits := detectOffset(target, p.numLeaves)
		if branchLen == 0 {
			// if there's no branch (1-tree) nothing to prove
			continue
		}
		node := p.roots[tNum]
		h := branchLen - 1
		pos := parentMany(target, branchLen, p.rows()) // this works but...
		// we should have a way to get the root positions from just p.roots

		lr := (bits >> h) & 1
		pos = (child(pos, p.rows())) | lr
		// descend until we hit the bottom, populating as we go
		// also populate siblings...
		for {
			if node.niece[lr] == nil {
				node.niece[lr] = new(polNode)
				node.niece[lr].data = proofMap[pos]
				if node.niece[lr].data == empty {
					return fmt.Errorf(
						"h %d wrote empty hash at pos %d %04x.niece[%d]",
						h, pos, node.data[:4], lr)
				}
				// fmt.Printf("h %d wrote %04x to %d\n", h, node.niece[lr].data[:4], pos)
				p.overWire++
			}
			if node.niece[lr^1] == nil {
				node.niece[lr^1] = new(polNode)
				node.niece[lr^1].data = proofMap[pos^1]
				// doesn't count as overwire because computed, not read
			}

			if h == 0 {
				break
			}
			h--
			node = node.niece[lr]
			lr = (bits >> h) & 1
			pos = (child(pos, p.rows()) ^ 2) | lr
		}

		// TODO do you need this at all?  If the Verify part already happened, maybe not?
		// at bottom, populate target if needed
		// if we don't need this and take it out, will need to change the forget
		// pop above

		if node.niece[lr^1] == nil {
			node.niece[lr^1] = new(polNode)
			node.niece[lr^1].data = proofMap[pos^1]
			fmt.Printf("------wrote %x at %d\n", proofMap[pos^1], pos^1)
			if node.niece[lr^1].data == empty {
				return fmt.Errorf("Wrote an empty hash h %d under %04x %d.niece[%d]",
					h, node.data[:4], pos, lr^1)
			}
			// p.overWire++ // doesn't count...? got it for free?
		}
	}
	return nil
}

// verifyBatchProof takes a block proof and reconstructs / verifies it.
// takes a blockproof to verify, and the known correct roots to check against.
// also takes the number of leaves and forest rows (those are redundant
// if we don't do weird stuff with overly-high forests, which we might)
// it returns a bool of whether the proof worked, and a map of the sparse
// forest in the blockproof
func (p *Pollard) verifyBatchProof(
	bp BatchProof, roots []Hash,
	numLeaves uint64, rows uint8) (bool, map[uint64]Hash) {

	// if nothing to prove, it worked
	if len(bp.Targets) == 0 {
		return true, nil
	}

	// Construct a map with positions to hashes
	proofmap, err := bp.Reconstruct(numLeaves, rows)
	if err != nil {
		fmt.Printf("VerifyBlockProof Reconstruct ERROR %s\n", err.Error())
		return false, proofmap
	}

	rootPositions, rootRows := getRootsReverse(numLeaves, rows)

	// partial forest is built, go through and hash everything to make sure
	// you get the right roots

	tagRow := bp.Targets
	nextRow := []uint64{}
	sortUint64s(tagRow) // probably don't need to sort

	// TODO it's ugly that I keep treating the 0-row as a special case,
	// and has led to a number of bugs.  It *is* special in a way, in that
	// the bottom row is the only thing you actually prove and add/delete,
	// but it'd be nice if it could all be treated uniformly.

	if verbose {
		fmt.Printf("tagrow len %d\n", len(tagRow))
	}

	var left, right uint64

	// iterate through rows
	for row := uint8(0); row <= rows; row++ {
		// iterate through tagged positions in this row
		for len(tagRow) > 0 {
			//nextRow = make([]uint64, len(tagRow))
			// Efficiency gains here. If there are two or more things to verify,
			// check if the next thing to verify is the sibling of the current leaf
			// we're on. Siblingness can be checked with bitwise XOR but since targets are
			// sorted, we can do bitwise OR instead.
			if len(tagRow) > 1 && tagRow[0]|1 == tagRow[1] {
				left = tagRow[0]
				right = tagRow[1]
				tagRow = tagRow[2:]
			} else { // if not only use one tagged position
				right = tagRow[0] | 1
				left = right ^ 1
				tagRow = tagRow[1:]
			}

			if verbose {
				fmt.Printf("left %d rootPoss %d\n", left, rootPositions[0])
			}
			// If the current node we're looking at this a root, check that
			// it matches the one we stored
			if left == rootPositions[0] {
				if verbose {
					fmt.Printf("one left in tagrow; should be root\n")
				}
				// Grab the received hash of this position from the map
				// This is the one we received from our peer
				computedRootHash, ok := proofmap[left]
				if !ok {
					fmt.Printf("ERR no proofmap for root at %d\n", left)
					return false, nil
				}
				// Verify that this root hash matches the one we stored
				if computedRootHash != roots[0] {
					fmt.Printf("row %d root, pos %d expect %04x got %04x\n",
						row, left, roots[0][:4], computedRootHash[:4])
					return false, nil
				}
				// otherwise OK and pop of the root
				roots = roots[1:]
				rootPositions = rootPositions[1:]
				rootRows = rootRows[1:]
				break
			}

			// Grab the parent position of the leaf we've verified
			parentPos := parent(left, rows)
			if verbose {
				fmt.Printf("%d %04x %d %04x -> %d\n",
					left, proofmap[left], right, proofmap[right], parentPos)
			}
			var parhash Hash
			n, _, _, err := p.readPos(parentPos)
			if err != nil {
				panic(err)
			}

			if n != nil && n.data != (Hash{}) {
				parhash = n.data
			} else {
				// this will crash if either is 0000
				// reconstruct the next row and add the parent to the map
				parhash = parentHash(proofmap[left], proofmap[right])
			}

			//parhash := parentHash(proofmap[left], proofmap[right])
			nextRow = append(nextRow, parentPos)
			proofmap[parentPos] = parhash
		}

		// Make the nextRow the tagRow so we'll be iterating over it
		// reset th nextRow
		tagRow = nextRow
		nextRow = []uint64{}

		// if done with row and there's a root left on this row, remove it
		if len(rootRows) > 0 && rootRows[0] == row {
			// bit ugly to do these all separately eh
			roots = roots[1:]
			rootPositions = rootPositions[1:]
			rootRows = rootRows[1:]
		}
	}

	return true, proofmap
}
