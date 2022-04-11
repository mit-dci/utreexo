package accumulator

import "fmt"

func (p *Pollard) addSwapless(adds []Leaf) {
	for _, add := range adds {
		node := new(polNode)
		node.data = add.Hash

		if p.full {
			node.remember = true
		} else {
			node.remember = add.Remember
		}

		// Add the hash to the map.
		if p.NodeMap != nil {
			p.NodeMap[add.Mini()] = node
		}

		// We can tell where the roots are by looking at the binary representation
		// of the numLeaves. Wherever there's a 1, there's a root.
		//
		// numLeaves of 8 will be '1000' in binary, so there will be one root at
		// row 3. numLeaves of 3 will be '11' in binary, so there's two roots. One at
		// row 0 and one at row 1.
		//
		// In this loop below, we're looking for these roots by checking if there's
		// a '1'. If there is a '1', we'll hash the root being added with that root
		// until we hit a '0'.
		for h := uint8(0); (p.numLeaves>>h)&1 == 1; h++ {
			// Grab and pop off the root that will become a node.
			root := p.roots[len(p.roots)-1]
			//p.roots = p.roots[:len(p.roots)-1]
			popSlice(&p.roots)

			// If the root that we're gonna hash with is empty, move the current
			// node up to the position of the parent.
			//
			// Example:
			//
			// 12
			// |-------\
			// 08      09
			// |---\   |---\
			// 00  01  02  03  --
			//
			// When we add 05 to this tree, 04 is empty so we move 05 to 10.
			// The resulting tree looks like below. The hash at position 10
			// is not hash(04 || 05) but just the hash of 05.
			//
			// 12
			// |-------\
			// 08      09      10
			// |---\   |---\   |---\
			// 00  01  02  03  --  --
			if root.data == empty {
				continue
			}

			// Roots point to their children. Those children become nieces here.
			//root.niece, node.niece = node.niece, root.niece
			root.leftNiece, root.rightNiece, node.leftNiece, node.rightNiece = node.leftNiece, node.rightNiece,
				root.leftNiece, root.rightNiece

			// Calculate the hash of the new root.
			nHash := parentHash(root.data, node.data)

			//node = &polNode{data: nHash, niece: [2]*polNode{root, node}}
			newRoot := &polNode{data: nHash, leftNiece: root, rightNiece: node}
			if p.full {
				newRoot.remember = true
			}

			// Set aunt.
			updateAunt(newRoot)
			newRoot.prune()
			node = newRoot
		}

		p.roots = append(p.roots, node)

		// Increment as we added a leaf.
		p.numLeaves++
	}
}

// updateAunt works its way down to the leaf node and updates the aunts for all the
// nieces.
func updateAunt(n *polNode) {
	if n.leftNiece != nil {
		// If the aunt is correct, we can return now as all nieces
		// of this niece will have the correct aunt.
		if n.leftNiece.aunt == n {
			return
		} else {
			// Update the aunt for this niece and check the nieces of this niece.
			n.leftNiece.aunt = n
			updateAunt(n.leftNiece)
		}
	}

	// Do the same for the other niece.
	if n.rightNiece != nil {
		if n.rightNiece.aunt == n {
			return
		} else {
			n.rightNiece.aunt = n
			updateAunt(n.rightNiece)
		}
	}
}

func delNode(node *polNode) {
	// Stop pointing to my aunt and make my aunt stop pointing at me.
	if node.aunt != nil {
		// It's ok if my aunt is not pointing at me because that means
		// it's already been updated.
		if node.aunt.rightNiece == node {
			node.aunt.rightNiece = nil
		} else if node.aunt.leftNiece == node {
			node.aunt.leftNiece = nil
		}
	}
	node.aunt = nil

	// Stop pointing to my leftNiece and make my leftNiece stop pointing at me.
	if node.leftNiece != nil {
		node.leftNiece.aunt = nil
	}
	node.leftNiece = nil

	// Same for right niece.
	if node.rightNiece != nil {
		node.rightNiece.aunt = nil
	}
	node.rightNiece = nil

	// Make myself nil.
	node = nil
}

func (p *Pollard) deleteNode(del uint64) error {
	//fmt.Printf("deleting %d\n", del)
	totalRows := treeRows(p.numLeaves)

	from := sibling(del)
	to := parent(del, totalRows)

	fromNode, fromNodeSib, _, err := p.readPos(from)
	if err != nil {
		return err
	}

	toNode, toSib, _, err := p.readPos(to)
	if err != nil {
		return err
	}

	if p.NodeMap != nil {
		delete(p.NodeMap, toNode.data.Mini())
	}

	//fmt.Printf("tonode %v, fromnode %v\n", toNode, fromNode)
	toNode.data = fromNode.data
	toSib.leftNiece, toSib.rightNiece = fromNodeSib.leftNiece, fromNodeSib.rightNiece

	// For GC.
	//if fromNode.leftNiece != nil {
	//	fromNode.leftNiece.aunt = nil
	//}
	//if fromNode.rightNiece != nil {
	//	fromNode.rightNiece.aunt = nil
	//}
	//fromNode.chop()
	//fromNode.aunt = nil
	//fromNode = nil
	delNode(fromNode)

	//if fromNodeSib.leftNiece != nil {
	//	fromNodeSib.leftNiece.aunt = nil
	//}
	//if fromNodeSib.rightNiece != nil {
	//	fromNodeSib.rightNiece.aunt = nil
	//}
	//fromNodeSib.chop()
	//fromNodeSib.aunt = nil

	delHash := fromNodeSib.data
	delNode(fromNodeSib)

	if p.NodeMap != nil {
		p.NodeMap[toNode.data.Mini()] = toNode
		delete(p.NodeMap, delHash.Mini())
	}

	updateAunt(toSib)

	// If to position is a root, there's no parent hash to be calculated.
	if isRootPosition(to, p.numLeaves, totalRows) {
		toNode.aunt = nil
		return nil
	}

	// TODO have the readPos also return the parent polnode. We can avoid this
	// extra read here.
	parentNode, parentSib, _, err := p.readPos(parent(to, totalRows))
	if err != nil {
		return err
	}

	toNode.aunt = parentSib

	var pHash Hash
	if isLeftChild(to) {
		pHash = parentHash(toNode.data, toSib.data)
	} else {
		pHash = parentHash(toSib.data, toNode.data)
	}
	parentNode.data = pHash

	return nil
}

func (p *Pollard) removeSwapless(dels []uint64) error {
	sortUint64s(dels)

	p.numDels += uint64(len(dels))

	totalRows := treeRows(p.numLeaves)
	deTwin(&dels, totalRows)

	fmt.Println("dels", dels)

	for _, del := range dels {
		// If a root is being deleted, then we mark it and all the leaves below
		// it to be deleted.
		if isRootPosition(del, p.numLeaves, totalRows) {
			node, _, _, err := p.grabPos(del)
			if err != nil {
				return err
			}

			if p.NodeMap != nil {
				delete(p.NodeMap, node.data.Mini())
			}

			idx := -1
			for i, root := range p.roots {
				if root.data == node.data {
					idx = i
				}
			}

			if p.roots[idx].leftNiece != nil {
				p.roots[idx].leftNiece.aunt = nil
			}
			if p.roots[idx].rightNiece != nil {
				p.roots[idx].rightNiece.aunt = nil
			}
			p.roots[idx].chop()
			p.roots[idx].aunt = nil
			p.roots[idx].data = empty
		} else {
			err := p.deleteNode(del)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (p *Pollard) ModifySwapless(adds []Leaf, delsOrig []uint64) error {
	dels := make([]uint64, len(delsOrig))
	copy(dels, delsOrig)
	sortUint64s(dels)

	//err := p.removeSwapless(delsOrig)
	err := p.removeSwaplessParallel(delsOrig)
	if err != nil {
		return err
	}

	p.addSwapless(adds)

	return nil
}

func (p *Pollard) MakeFull() {
	p.NodeMap = make(map[MiniHash]*polNode)
	p.full = true
}

func (p *Pollard) Verify(proof BatchProof) {
}

//func (p *Pollard) readPosition(pos uint64) (n, sibling, parent *polNode, err error) {
//	// Tree is the root the position is located under.
//	// branchLen denotes how far down the root the position is.
//	// bits tell us if we should go down to the left child or the right child.
//	tree, branchLen, bits := detectOffset(pos, p.numLeaves)
//
//	// Initialize all 3 to the root.
//	n, sibling, parent = p.roots[tree], p.roots[tree], p.roots[tree]
//
//	// branchLen of 0 means that we want h
//	if branchLen == 0 {
//		return
//	}
//
//	for h := branchLen; h > 0; h-- {
//		right := uint8(bits>>h) & 1
//	}
//
//	return
//}
