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
	polNodeCount := 0
	for _, tree := range trees {
		polNodeCount += len(tree)
	}
	polNodes := make([]polNode, polNodeCount*3)

	// rootIdx and rootIdxBackwards is needed because p.populate()
	// expects the roots in a reverse order. Thus the need for two
	// indexes. TODO fix this to have only one index
	rootIdx := len(rootHashes) - 1
	rootIdxBackwards := 0
	nodesAllocated := 0
	rows := p.rows()
	for _, root := range roots {
		for root.Val != rootHashes[rootIdx] {
			rootIdx--
			rootIdxBackwards++
		}
		// populate the pollard
		nodesAllocated += populate(rows, root.Pos, p.roots[(len(p.roots)-rootIdxBackwards)-1],
			&trees[len(p.roots)-rootIdxBackwards-1], polNodes[nodesAllocated:])
	}

	return nil
}

// nodesToFollow returns the positions of the nodes for the branch you want to go to,
// that are needed to populate all of the given trees.
//
// The passed in trees MUST be in ascending order (by parent.Pos).
func nodesToFollow(trees []miniTree, branchToGoTo, rows uint8) []uint64 {
	// if there's nothing to populate, return early
	if len(trees) <= 0 {
		return []uint64{}
	}
	// 1<<branchToGoTo (aka 2**branchToGoTo) gives the maximum possible nodes for this branch
	// Example:
	//
	// branchLen: 0  14
	//               |---------------\
	// branchLen: 1  12              13
	//               |-------\       |-------\
	// branchLen: 2  08      09      10      11
	//               |---\   |---\   |---\   |---\
	// branchLen: 3  00  01  02  03  04  05  06  07
	//
	// Fairly trivial to see that 1<<0 is the root so max node of 1. 1<<3 is 8 so
	// max node of 8.
	maxNodesAtBranch := 1 << branchToGoTo

	// Just for checking if a node to follow is a duplicate or not
	// There's probably a better way to check for duplicates.
	existsMap := make(map[uint64]struct{}, len(trees)/2)

	// sometimes this might not be full and we might be over allocating
	nodesToFollow := make([]uint64, 0, len(trees))

	for i := 0; i < len(trees); i++ {
		// If we've already found the maximum that we could, break.
		// There are many cases where you follow all the nodes at the current row
		// so this happens often.
		if len(nodesToFollow) >= maxNodesAtBranch {
			break
		}
		// This should never happen. The passed in trees have already been
		// verified by verifyBatchProof so the trees are correct and should
		// be in ascending order.
		if trees[i].branchLen < branchToGoTo {
			err := fmt.Errorf("Attempt to fetch a node that's above the requested row. "+
				"Passed in a tree with branchLen of %d "+
				"which is less than the requested branchToGoTo of %d\n",
				trees[i].branchLen, branchToGoTo)
			panic(err)
		}

		rise := trees[i].branchLen - branchToGoTo
		node := parentMany(trees[i].parent.Pos, rise, rows)

		// only append if it's unique
		_, exists := existsMap[node]
		if !exists {
			// also need to grab the siblings since polNodes point to
			// nieces not children (not roots though roots point to their children).
			// NOTE we are wasting some space here but it's not much so eh
			nodeSib := node ^ 1

			// append in ascending order
			if nodeSib < node {
				nodesToFollow = append(nodesToFollow, nodeSib)
				nodesToFollow = append(nodesToFollow, node)
			} else {
				nodesToFollow = append(nodesToFollow, node)
				nodesToFollow = append(nodesToFollow, nodeSib)
			}
			existsMap[node] = struct{}{}
			existsMap[nodeSib] = struct{}{}

			// look ahead for trees with same ancestors and skip those nodes.
			// This saves on map access.
			skip := 0
			// if rise == 0, don't skip since there's no ancestors
			if rise != 0 {
				// start at i+1 to skip the node that we just processed above.
				// Loop until either:
				// 1) We've looked at all the trees at need to be populated
				// 2) We're done looking through this row
				// 3) We've finished finding all nodes that share an ancestor
				//    for this row.
				for j := i + 1; j < len(trees); j++ {
					// Case (2). If the tree we're at is of a different branch, then break
					// since it (and all nodes after) cannot be a cousin or a sibling
					if trees[j].branchLen != trees[i].branchLen {
						break
					}
					ancestor := parentMany(trees[i].parent.Pos, rise, rows)

					// Case (3). Since nodes are in ascending order and we're
					// only looking at a single row, if the ancestor is
					// different, all the trees preceeding this one is
					// also gonna have a different ancestor.
					if ancestor == node {
						skip++
					} else {
						break
					}
				}
			}
			// Add to i to actually skip
			i += skip
		}
	}

	return nodesToFollow
}

// polNodeAndPos is just a polNode and its position in as a struct
type polNodeAndPos struct {
	node *polNode
	pos  uint64
}

// nextNodes returns a slice of nodes on the row below the curBranch that need
// to be followed to populate every miniTree in the given slice of trees. The
// nodes returned are in ascending order.
//
// curNodes and trees (by parent pos for trees) passed to this function MUST be
// in ascending order. curNodes also must not start at the root.
func nextNodes(curBranch, rows uint8, curNodes []*polNodeAndPos, trees []miniTree) []*polNodeAndPos {
	// No nextNodes if there's no more trees to be populated
	if len(trees) == 0 {
		return []*polNodeAndPos{}
	}

	// curBranch+1 as we want to go one row below. Branch is "how far down are we from
	// the root". This is easy to see from the visualization with the tree blow.
	//
	// branchLen: 0  14
	//               |---------------\
	// branchLen: 1  12              13
	//               |-------\       |-------\
	// branchLen: 2  08      09      10      11
	//               |---\   |---\   |---\   |---\
	// branchLen: 3  00  01  02  03  04  05  06  07
	nextNodes := nodesToFollow(trees, curBranch+1, rows)
	nextCurNodes := make([]*polNodeAndPos, 0, len(nextNodes))

	// Both curNodes and nextNodes are in ascending order. We keep an index for both
	// and increment nextNodesIdx as we process each nextNode. Since both are in ascending
	// order, we're guaranteed to have the nextCurNodes slice also be in order.
	//
	// Only exception is if we have twins. For twins, we need to grab the right sibling
	// first then the left sibling. As you can see in the tree below, 09 points to 00 and 01
	// and 08 points to 02 and 03. Thus we need to grab 09's nieces and then grab 08's nieces.
	//
	//  08      09      10      11
	//  |---\   |---\   |---\   |---\
	//  00  01  02  03  04  05  06  07
	nextNodesIdx := 0
	for i := 0; i < len(curNodes); i++ {
		if nextNodesIdx >= len(nextNodes) {
			break
		}

		// Check for twins
		extraNodesToProcess := 0
		if i+1 < len(curNodes) &&
			curNodes[i].pos|1 == curNodes[i+1].pos {
			extraNodesToProcess++
		}

		// A loop for processing either twins or just a single node
		// if we're processing twins, we grab i+1 and then i
		// if not, we're just grabbing i
		for idx := i + extraNodesToProcess; idx >= i; idx-- {
			if nextNodesIdx >= len(nextNodes) {
				break
			}
			curNode := curNodes[idx]

			// pos^1 gives the sibling's pos. Then grab the pos of my sibling's
			// chilren (my sibling's chilren are my nieces). Isn't genealogy fun?
			lNiecePos := child(curNode.pos^1, rows)
			rNiecePos := lNiecePos | 1

			// Check if they exist
			lExists := nextNodes[nextNodesIdx] == lNiecePos
			rExists := nextNodes[nextNodesIdx+1] == rNiecePos

			// Append in ascending order. Left first then right
			if lExists {
				nextNodesIdx++
				// Should never happen but keep the check in there
				// for now
				if curNode.node.niece[0] == nil {
					curNode.node.niece[0] = &polNode{}
				}
				nextCurNodes = append(nextCurNodes,
					&polNodeAndPos{curNode.node.niece[0], lNiecePos})
			}
			if rExists {
				nextNodesIdx++
				// Should never happen but keep the check in there
				// for now.
				if curNode.node.niece[1] == nil {
					curNode.node.niece[1] = &polNode{}
				}
				nextCurNodes = append(nextCurNodes,
					&polNodeAndPos{curNode.node.niece[1], rNiecePos})
			}
		}

		if extraNodesToProcess > 0 {
			// increment one more since we processed two nodes
			i++
		}
	}

	return nextCurNodes
}

// Given a single miniTree and a single aunt (aka sibling of the miniTree.parent),
// populateOne populates the niceces with the children of the tree with
// the passed in polNodes.
func populateOne(tree miniTree, node *polNode, polNodes []polNode) int {
	nodesAllocated := 0

	if node.niece[0] == nil {
		node.niece[0] = &polNodes[nodesAllocated]
		nodesAllocated++
	}
	node.niece[0].data = tree.leftChild.Val

	if node.niece[1] == nil {
		node.niece[1] = &polNodes[nodesAllocated]
		nodesAllocated++
	}
	node.niece[1].data = tree.rightChild.Val

	return nodesAllocated
}

// populate takes a root and populates it with the nodes of the paritial proof tree that was computed
// in `verifyBatchProof`. `trees` being passed in must start from the root. polNodes is the nodes
// that we'll be populating into. Length of polNodes MUST match all the nodes that will be populated.
//
// populate returns how many polNodes have been populated.
func populate(rows uint8, pos uint64, root *polNode, trees *[]miniTree, polNodes []polNode) int {
	// If there's nothing to populate, return early
	if len(*trees) <= 0 {
		return 0
	}

	nodesAllocated := 0

	// treat root as special since it points to children not niceces. Populate these first.
	nodesAllocated += populateOne((*trees)[len(*trees)-1], root, polNodes[nodesAllocated:])

	// pop off root
	*trees = (*trees)[:len(*trees)-1]

	// Append the root's children to curNodes. We start populating from the root's children
	curNodes := make([]*polNodeAndPos, 0, 2)
	curNodes = append(curNodes, &polNodeAndPos{root.niece[0], child(pos, rows)})
	curNodes = append(curNodes, &polNodeAndPos{root.niece[1], child(pos, rows) | 1})

	// populate all the trees passed in.
	for len(*trees) > 0 {
		curBranchLen := int((*trees)[len(*trees)-1].branchLen)
		curNodeIdx := len(curNodes) - 1

		// populate all the trees for this row (nodes on the same row have the same
		// branchLen).
		for {
			// Break if any of these 3 conditions are met:
			// 1) We finished populating all the trees.
			// 2) We finished processing all the curNodes.
			// 3) We're finished processing all the trees for this row.
			if len(*trees) <= 0 || curNodeIdx < 0 ||
				int((*trees)[len(*trees)-1].branchLen) != curBranchLen {
				break
			}

			// check if the last and second to last are twins
			// only check if there are two elements in trees
			isTwin := false
			if len(*trees) > 1 {
				isTwin = (*trees)[len(*trees)-2].parent.Pos|1 ==
					(*trees)[len(*trees)-1].parent.Pos
			}

			if isTwin {
				// If the 2 next trees in queue are twins but they don't correspond with
				// curNodes, decrement curNodes and continue.
				if (*trees)[len(*trees)-1].parent.Pos^1 != curNodes[curNodeIdx-1].pos ||
					(*trees)[len(*trees)-2].parent.Pos^1 != curNodes[curNodeIdx].pos {
					curNodeIdx--
					continue
				}
				left := (*trees)[len(*trees)-2]
				nodeForLeft := curNodes[curNodeIdx]
				curNodeIdx--
				nodesAllocated += populateOne(left, nodeForLeft.node, polNodes[nodesAllocated:])

				right := (*trees)[len(*trees)-1]
				nodeForRight := curNodes[curNodeIdx]
				curNodeIdx--
				nodesAllocated += populateOne(right, nodeForRight.node, polNodes[nodesAllocated:])

				// pop off 2 since we just processed two
				*trees = (*trees)[:len(*trees)-2]
				continue
			}

			tree := (*trees)[len(*trees)-1]
			curNode := curNodes[curNodeIdx]

			if curNode.pos == tree.parent.Pos^1 {
				nodesAllocated += populateOne(tree, curNode.node, polNodes[nodesAllocated:])
				*trees = (*trees)[:len(*trees)-1]
			}

			// we couldn't match anything. Move on to the next curNode
			curNodeIdx--
		}

		nextCurNodes := nextNodes(uint8(curBranchLen), rows, curNodes, *trees)
		curNodes = nextCurNodes
	}

	return nodesAllocated
}
