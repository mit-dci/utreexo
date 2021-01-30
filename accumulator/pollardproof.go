package accumulator

import (
	"bytes"
	"fmt"
)

// IngestBatchProof populates the Pollard with all needed data to delete the
// targets in the block proof
func (p *Pollard) IngestBatchProof(bp BatchProof, targetHashes []Hash) error {

	// first, save the rootHashes.  If ingestAndCheck fails, the pollard
	// will be messed up / invalid, and we can wipe everything and restore
	// to the roots before we ingested.  (not idea but works for now)
	// TODO: cleaner failure mode for ingesting a bad proof

	var buf bytes.Buffer
	p.WritePollard(&buf)

	err := p.ingestAndCheck(bp, targetHashes)
	if err != nil {
		fmt.Printf("ingest proof failure: %s restoring pollard\n", err.Error())
		p.RestorePollard(&buf)
		return fmt.Errorf("Invalid proof, pollard wiped down to roots")
	}
	return nil
}

// ingestAndCheck puts the targets and proofs from the BatchProof into the
// pollard, and computes parents as needed up to already populated nodes.
func (p *Pollard) ingestAndCheck(bp BatchProof, targs []Hash) error {
	if len(targs) == 0 {
		return nil
	}
	fmt.Printf("got proof %s\n", bp.ToString())

	// the main thing ingestAndCheck does is write hashes to the pollard.
	// the hashes can come from 2 places: arguments or hashing.
	// for arguments, proofs and targets are treated pretty much the same;
	// read em off the slice and write em in.
	// any time you're writing somthing that's already there, check to make
	// sure it matches.  if it doesn't, return an error.
	// if it does, you don't need to hash any parents above that.

	// first range through targets, populating / matching, and placing proof
	// hashes if the targets are not twins

	for i, targpos := range bp.Targets {

	}

	return nil
}

// populate takes a root and populates it with the nodes of the paritial proof
// tree that was computed in `verifyBatchProof`.
func (p *Pollard) populate(
	root *polNode, pos uint64, trees []miniTree, polNodes []polNode) int {
	// a stack to traverse the pollard
	type stackElem struct {
		trees []miniTree
		node  *polNode
		pos   uint64
	}
	stack := make([]stackElem, 0, len(trees))
	stack = append(stack, stackElem{trees, root, pos})
	rows := p.rows()
	nodesAllocated := 0
	for len(stack) > 0 {
		elem := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if elem.pos < p.numLeaves {
			// this is a leaf, we are done populating this branch.
			continue
		}

		leftChild := child(elem.pos, rows)
		rightChild := child(elem.pos, rows) | 1
		var left, right *polNode
		i := len(elem.trees) - 1
	find_nodes:
		for ; i >= 0; i-- {
			switch elem.trees[i].parent.Pos {
			case elem.pos:
				fallthrough
			case rightChild:
				if elem.node.niece[0] == nil {
					elem.node.niece[0] = &polNodes[nodesAllocated]
					nodesAllocated++
				}
				right = elem.node.niece[0]
				right.data = elem.trees[i].l.Val
				fallthrough
			case leftChild:
				if elem.node.niece[1] == nil {
					elem.node.niece[1] = &polNodes[nodesAllocated]
					nodesAllocated++
				}
				left = elem.node.niece[1]
				left.data = elem.trees[i].r.Val
				break find_nodes
			}
		}
		if i < 0 {
			continue
		}

		stack = append(stack,
			stackElem{trees[:i], left, leftChild},
			stackElem{trees[:i], right, rightChild})
	}
	return nodesAllocated
}
