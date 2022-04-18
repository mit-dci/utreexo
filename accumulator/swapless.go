package accumulator

import (
	"encoding/hex"
	"fmt"
	"sort"
)

func (p *Pollard) addSwapless(adds []Leaf) {
	for _, add := range adds {
		node := new(polNode)
		node.data = add.Hash

		if p.full {
			add.Remember = true
		}
		node.remember = add.Remember

		// Add the hash to the map.
		if p.NodeMap != nil && add.Remember {
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

// updateAunt works its way down, updating the aunts for all the nieces until it
// encounters the first niece that has the correct aunt.
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
		// Figure out if this node is the left or right niece and make that nil.
		if node.aunt.rightNiece == node {
			node.aunt.rightNiece = nil
		} else if node.aunt.leftNiece == node {
			node.aunt.leftNiece = nil
		} else {
			// Purposely left empty. It's ok if my aunt is not pointing
			// at me because that means it's already been updated.
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

	fromNode, fromNodeSib, _, err := p.readPosition(from)
	if err != nil {
		return err
	}

	toNode, toSib, parentNode, err := p.readPosition(to)
	if err != nil {
		return err
	}

	if p.NodeMap != nil {
		delete(p.NodeMap, toNode.data.Mini())
	}

	//fmt.Printf("tonode %v, fromnode %v\n", toNode, fromNode)
	toNode.data = fromNode.data
	toSib.leftNiece, toSib.rightNiece = fromNodeSib.leftNiece, fromNodeSib.rightNiece

	//fromNode.leftNiece, fromNode.rightNiece = toNode.leftNiece, toNode.rightNiece
	//aunt := toNode.aunt
	//if aunt != nil {
	//	if aunt.leftNiece == toNode {
	//		aunt.leftNiece = fromNode
	//	} else if aunt.rightNiece == toNode {
	//		aunt.rightNiece = fromNode
	//	} else {
	//		return fmt.Errorf("Node with hash %s has an incorrect aunt pointer "+
	//			"or the aunt with hash %s has incorrect pointer to its nieces",
	//			hex.EncodeToString(toNode.data[:]), hex.EncodeToString(aunt.data[:]))
	//	}
	//}
	//fromNode.aunt = aunt

	delHash := fromNodeSib.data
	// For GC.
	delNode(fromNode)
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

	// Set aunt.
	toNode.aunt, err = parentNode.getSibling()
	if err != nil {
		return err
	}
	// If there's no sibling, it means that toNode is a children of
	// the root.
	if toNode.aunt == nil {
		toNode.aunt = parentNode
	}

	// Hash until we get to the root.
	for parentNode != nil {
		// Grab children of this parent.
		leftChild, rightChild, err := parentNode.getChildren()
		if err != nil {
			return err
		}

		parentNode.data = parentHash(leftChild.data, rightChild.data)

		// Grab the next parent that needs the hash updated.
		parentNode, err = parentNode.getParent()
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *Pollard) removeSwapless(dels []uint64) error {
	sortUint64s(dels)

	p.numDels += uint64(len(dels))

	totalRows := treeRows(p.numLeaves)
	deTwin(&dels, totalRows)

	//fmt.Println("dels", dels)

	for _, del := range dels {
		// If a root is being deleted, then we mark it and all the leaves below
		// it to be deleted.
		if isRootPosition(del, p.numLeaves, totalRows) {
			tree, _, _ := detectOffset(del, p.numLeaves)
			if tree >= uint8(len(p.roots)) {
				return ErrorStrings[ErrorNotEnoughTrees]
			}

			// Delete from map.
			if p.NodeMap != nil {
				delete(p.NodeMap, p.roots[tree].data.Mini())
			}

			if p.roots[tree].leftNiece != nil {
				p.roots[tree].leftNiece.aunt = nil
			}
			if p.roots[tree].rightNiece != nil {
				p.roots[tree].rightNiece.aunt = nil
			}
			p.roots[tree].chop()
			p.roots[tree].aunt = nil
			p.roots[tree].data = empty
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

	err := p.removeSwapless(delsOrig)
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

type hashAndPos struct {
	hash Hash
	pos  uint64
}

type nodeAndPos struct {
	node *polNode
	pos  uint64
}

func sortHashAndPos(s []hashAndPos) {
	sort.Slice(s, func(a, b int) bool { return s[a].pos < s[b].pos })
}

//func calcRemoveIdx(delHashes []Hash, proof BatchProof, treeRows uint8) {
//	for h := 0; h < int(treeRows); h++ {
//		detectRow()
//	}
//}

func (p *Pollard) Verify2(delHashes []Hash, proof BatchProof) error {
	toProve := make([]nodeAndPos, len(delHashes))

	for i, delHash := range delHashes {
		node, found := p.NodeMap[delHash.Mini()]
		if found {
			toProve[i] = nodeAndPos{node, proof.Targets[i]}
		} else {
			n := &polNode{data: delHash, remember: true}
			toProve[i] = nodeAndPos{n, proof.Targets[i]}
		}
	}

	return nil
}

func (p *Pollard) Verify(delHashes []Hash, proof BatchProof) error {
	if len(delHashes) == 0 {
		return nil
	}
	//fmt.Println("targets", proof.Targets)
	toProve := make([]hashAndPos, len(delHashes))

	for i := range toProve {
		toProve[i].hash = delHashes[i]
		toProve[i].pos = proof.Targets[i]
	}

	sortHashAndPos(toProve)

	rootHashes := p.calculateRoots(toProve, proof.Proof)
	if len(rootHashes) == 0 {
		return fmt.Errorf("No roots calculated but has %d deletions", len(delHashes))
	}

	for i, rootHash := range rootHashes {
		fmt.Printf("root %d, hash %s\n", i, hex.EncodeToString(rootHash[:]))
	}

	rootMatches := 0
	for i := range p.roots {
		if len(rootHashes) > rootMatches &&
			p.roots[len(p.roots)-(i+1)].data == rootHashes[rootMatches] {
			rootMatches++
		}
	}
	if len(rootHashes) != rootMatches {
		// the proof is invalid because some root candidates were not
		// included in `roots`.
		err := fmt.Errorf("Pollard.Verify: generated %d roots but only"+
			"matched %d roots", len(rootHashes), rootMatches)
		return err
	}

	return nil
}

func (p *Pollard) calculateRoots(toProve []hashAndPos, proofHashes []Hash) []Hash {
	calculatedRootHashes := make([]Hash, 0, len(p.roots))
	totalRows := treeRows(p.numLeaves)

	var nextProves []hashAndPos
	for row := 0; row <= int(totalRows); row++ {
		extractedProves := extractRowHash(toProve, totalRows, uint8(row))

		proves := mergeSortedHashAndPos(nextProves, extractedProves)
		nextProves = nextProves[:0]

		for i := 0; i < len(proves); i++ {
			prove := proves[i]

			// This means we hashed all the way to the top of this subtree.
			if isRootPosition(prove.pos, p.numLeaves, totalRows) {
				calculatedRootHashes = append(calculatedRootHashes, prove.hash)
				continue
			}

			// Check if the next prove is the sibling of this prove.
			if i+1 < len(proves) && rightSib(prove.pos) == proves[i+1].pos {
				nextProve := hashAndPos{
					hash: parentHash(prove.hash, proves[i+1].hash),
					pos:  parent(prove.pos, totalRows),
				}
				nextProves = append(nextProves, nextProve)

				i++ // Increment one more since we procesed another prove.
			} else {
				hash := proofHashes[0]
				proofHashes = proofHashes[1:]

				nextProve := hashAndPos{pos: parent(prove.pos, totalRows)}
				if isLeftChild(prove.pos) {
					nextProve.hash = parentHash(prove.hash, hash)
				} else {
					nextProve.hash = parentHash(hash, prove.hash)
				}

				nextProves = append(nextProves, nextProve)
			}
		}
	}

	return calculatedRootHashes
}

func (p *Pollard) VerifyCached(delHashes []Hash, proof BatchProof) error {
	if len(delHashes) == 0 {
		return nil
	}
	//fmt.Println("targets", proof.Targets)
	toProve := make([]hashAndPos, len(delHashes))

	for i := range toProve {
		_, found := p.NodeMap[delHashes[i].Mini()]
		if found {
			//if node.data != delHashes[i] {
			//	// Should never happen as this would mean that the
			//	// nodeMap is wrong.
			//	return fmt.Errorf("Passed hash of ")
			//}

			toProve[i].hash = empty
			toProve[i].pos = proof.Targets[i]
		} else {
			toProve[i].hash = delHashes[i]
			toProve[i].pos = proof.Targets[i]
		}
	}

	sortHashAndPos(toProve)

	rootHashes, err := p.calculateRootsCached(toProve, proof.Proof)
	if err != nil {
		return err
	}
	if len(rootHashes) == 0 {
		return fmt.Errorf("No roots calculated but has %d deletions", len(delHashes))
	}

	//for i, rootHash := range rootHashes {
	//	fmt.Printf("root %d, hash %s\n", i, hex.EncodeToString(rootHash[:]))
	//}

	rootMatches := 0
	for i := range p.roots {
		if len(rootHashes) > rootMatches &&
			p.roots[len(p.roots)-(i+1)].data == rootHashes[rootMatches] {
			rootMatches++
		}
	}
	if len(rootHashes) != rootMatches {
		// the proof is invalid because some root candidates were not
		// included in `roots`.
		err := fmt.Errorf("Pollard.Verify: generated %d roots but only"+
			"matched %d roots", len(rootHashes), rootMatches)
		return err
	}

	return nil
}

func (p *Pollard) calculateRootsCached(toProve []hashAndPos, proofHashes []Hash) ([]Hash, error) {
	calculatedRootHashes := make([]Hash, 0, len(p.roots))
	totalRows := treeRows(p.numLeaves)

	var nextProves []hashAndPos
	for row := 0; row <= int(totalRows); row++ {
		extractedProves := extractRowHash(toProve, totalRows, uint8(row))

		proves := mergeSortedHashAndPos(nextProves, extractedProves)
		nextProves = nextProves[:0]

		for i := 0; i < len(proves); i++ {
			prove := proves[i]

			// This means we hashed all the way to the top of this subtree.
			if isRootPosition(prove.pos, p.numLeaves, totalRows) {
				if prove.hash == empty {
					tree, _, _ := detectOffset(prove.pos, p.numLeaves)
					calculatedRootHashes = append(calculatedRootHashes, p.roots[tree].data)
					//fmt.Println("cached root")
				} else {
					calculatedRootHashes = append(calculatedRootHashes, prove.hash)
					//fmt.Println("non-cached root")
				}
				continue
			}

			// Check if the next prove is the sibling of this prove.
			if i+1 < len(proves) && rightSib(prove.pos) == proves[i+1].pos {
				var pHash Hash
				if prove.hash != empty && proves[i+1].hash != empty {
					pHash = parentHash(prove.hash, proves[i+1].hash)
				} else {
					if prove.hash != empty {
						n, _, _, err := p.readPosition(prove.pos)
						if err != nil {
							return nil, err
						}

						if n.data != prove.hash {
							err := fmt.Errorf("Position %d, has cached hash %s "+
								"but got calculated hash of %s ", prove.pos,
								hex.EncodeToString(n.data[:]),
								hex.EncodeToString(prove.hash[:]))
							return nil, err
						}
					} else if proves[i+1].hash != empty {
						n, _, _, err := p.readPosition(proves[i+1].pos)
						if err != nil {
							return nil, err
						}

						if n.data != proves[i+1].hash {
							err := fmt.Errorf("Position %d, has cached hash %s "+
								"but got calculated hash of %s ", prove.pos,
								hex.EncodeToString(n.data[:]),
								hex.EncodeToString(proves[i+1].hash[:]))
							return nil, err
						}
					} else {
						// If both are empty than the hashes are cached and
						// verified.
					}

					pHash = empty
				}
				nextProve := hashAndPos{
					//hash: parentHash(prove.hash, proves[i+1].hash),
					hash: pHash,
					pos:  parent(prove.pos, totalRows),
				}
				nextProves = append(nextProves, nextProve)
				//fmt.Printf("skipping sib %s\n", hex.EncodeToString(proves[i+1].hash[:]))

				i++ // Increment one more since we procesed another prove.
			} else {
				hash := proofHashes[0]
				proofHashes = proofHashes[1:]

				nextProve := hashAndPos{pos: parent(prove.pos, totalRows)}
				if prove.hash == empty {
					nextProve.hash = empty
				} else {
					if isLeftChild(prove.pos) {
						nextProve.hash = parentHash(prove.hash, hash)
					} else {
						nextProve.hash = parentHash(hash, prove.hash)
					}
				}

				nextProves = append(nextProves, nextProve)
			}
			//fmt.Printf("Proving %s, got parent %s\n",
			//	hex.EncodeToString(prove.hash[:]),
			//	hex.EncodeToString(nextProves[len(nextProves)-1].hash[:]))
		}
	}

	return calculatedRootHashes, nil
}

func mergeSortedHashAndPos(a []hashAndPos, b []hashAndPos) (c []hashAndPos) {
	maxa := len(a)
	maxb := len(b)

	// shortcuts:
	if maxa == 0 {
		return b
	}
	if maxb == 0 {
		return a
	}

	// make it (potentially) too long and truncate later
	c = make([]hashAndPos, maxa+maxb)

	idxa, idxb := 0, 0
	for j := 0; j < len(c); j++ {
		// if we're out of a or b, just use the remainder of the other one
		if idxa >= maxa {
			// a is done, copy remainder of b
			j += copy(c[j:], b[idxb:])
			c = c[:j] // truncate empty section of c
			break
		}
		if idxb >= maxb {
			// b is done, copy remainder of a
			j += copy(c[j:], a[idxa:])
			c = c[:j] // truncate empty section of c
			break
		}

		vala, valb := a[idxa], b[idxb]
		if vala.pos < valb.pos { // a is less so append that
			c[j] = vala
			idxa++
		} else if vala.pos > valb.pos { // b is less so append that
			c[j] = valb
			idxb++
		} else { // they're equal
			c[j] = vala
			idxa++
			idxb++
		}
	}
	return
}

func extractRowHash(toProve []hashAndPos, forestRows, rowToExtract uint8) []hashAndPos {
	if len(toProve) < 0 {
		return []hashAndPos{}
	}

	start := -1
	end := 0

	for i := 0; i < len(toProve); i++ {
		if detectRow(toProve[i].pos, forestRows) == rowToExtract {
			if start == -1 {
				start = i
			}

			end = i
		} else {
			// If we're not at the desired row and start has already been set
			// once, that means we've extracted everything we can. This is
			// possible because the assumption is that the toProve are sorted.
			if start != -1 {
				break
			}
		}
	}

	if start == -1 {
		return []hashAndPos{}
	}

	count := (end + 1) - start
	row := make([]hashAndPos, count)

	copy(row, toProve[start:end+1])

	return row
}

func (p *Pollard) readPosition(pos uint64) (n, sibling, parent *polNode, err error) {
	// Tree is the root the position is located under.
	// branchLen denotes how far down the root the position is.
	// bits tell us if we should go down to the left child or the right child.
	tree, branchLen, bits := detectOffset(pos, p.numLeaves)

	// Roots point to their children so we actually need to bitflip this.
	// TODO fix detectOffset.
	bits ^= 1

	// Initialize.
	n, sibling, parent = p.roots[tree], p.roots[tree], nil

	// Go down the tree to find the node we're looking for.
	for h := int(branchLen) - 1; h >= 0; h-- {
		// Parent is the sibling of the current node as each of the
		// nodes point to their nieces.
		parent = sibling

		// Figure out which node we need to follow.
		niecePos := uint8(bits>>h) & 1
		if isLeftChild(uint64(niecePos)) {
			n, sibling = n.leftNiece, n.rightNiece
		} else {
			n, sibling = n.rightNiece, n.leftNiece
		}

		// Return early if the path to the node we're looking for
		// doesn't exist.
		if n == nil {
			return nil, nil, nil, nil
		}
	}

	return
}
