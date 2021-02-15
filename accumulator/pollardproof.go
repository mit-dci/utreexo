package accumulator

import (
	"fmt"
)

// IngestBatchProof populates the Pollard with all needed data to delete the
// targets in the block proof
func (p *Pollard) IngestBatchProof(bp BatchProof) error {
	// verify the batch proof.
	rootHashes := p.rootHashesForward()
	ok, trees, roots := verifyBatchProof(bp, rootHashes, p.numLeaves,
		// pass a closure that checks the pollard for cached nodes.
		// returns true and the hash value of the node if it exists.
		// returns false if the node does not exist or the hash value is empty.
		func(pos uint64) (bool, Hash) {
			n, _, _, err := p.readPos(pos)
			if err != nil {
				return false, empty
			}
			if n != nil && n.data != empty {
				return true, n.data
			}

			return false, empty
		})
	if !ok {
		return fmt.Errorf("block proof mismatch")
	}

	// preallocating polNodes helps with garbage collection
	polNodes := make([]polNode, len(trees)*3)

	// rootIdx and rootIdxBackwards is needed because p.populate()
	// expects the roots in a reverse order. Thus the need for two
	// indexes. TODO fix this to have only one index
	rootIdx := len(rootHashes) - 1
	rootIdxBackwards := 0
	nodesAllocated := 0
	for _, root := range roots {
		for root.Val != rootHashes[rootIdx] {
			rootIdx--
			rootIdxBackwards++
		}
		// populate the pollard
		nodesAllocated += p.populate(p.roots[(len(p.roots)-rootIdxBackwards)-1],
			root.Pos, trees, polNodes[nodesAllocated:])
	}

	return nil
}

// populate takes a root and populates it with the nodes of the paritial proof tree that was computed
// in `verifyBatchProof`.
func (p *Pollard) populate(root *polNode, pos uint64, trees [][3]node, polNodes []polNode) int {
	// a stack to traverse the pollard
	type stackElem struct {
		trees [][3]node
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
			switch elem.trees[i][0].Pos {
			case elem.pos:
				fallthrough
			case rightChild:
				if elem.node.niece[0] == nil {
					elem.node.niece[0] = &polNodes[nodesAllocated]
					nodesAllocated++
				}
				right = elem.node.niece[0]
				right.data = elem.trees[i][1].Val
				fallthrough
			case leftChild:
				if elem.node.niece[1] == nil {
					elem.node.niece[1] = &polNodes[nodesAllocated]
					nodesAllocated++
				}
				left = elem.node.niece[1]
				left.data = elem.trees[i][2].Val
				break find_nodes
			}
		}
		if i < 0 {
			continue
		}

		stack = append(stack,
			stackElem{trees[:i], left, leftChild}, stackElem{trees[:i], right, rightChild})
	}
	return nodesAllocated
}
