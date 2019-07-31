package utreexo

// Pollard is the sparse representation of the utreexo forest, using
// binary tree pointers instead of a hash map.

// I generally avoid recursion as much as I can, using regular for loops and
// ranges instead.  That might start looking pretty contrived here, but
// I'm still going to try it.

// Pollard :
type Pollard struct {
	numLeaves uint64 // number of leaves in the pollard forest

	tops []*polNode // slice of the tree tops, which are polNodes.
	// tops are in big to small order
	// BUT THEY'RE WEIRD!  The left / right children are actual children,
	// not neices as they are in every lower level.

	hashesEver, rememberEver, overWire uint64

	//	Lookahead int32  // remember leafs below this TTL
	//	Minleaves uint64 // remember everything below this leaf count
}

// PolNode is a node in the pollard forest
type polNode struct {
	data  Hash
	niece [2]*polNode
}

// auntOp returns the hash of a nodes neices. crashes if you call on nil neices.
func (n *polNode) auntOp() Hash {
	return Parent(n.niece[0].data, n.niece[1].data)
}

// auntOp tells you if you can call auntOp on a node
func (n *polNode) auntable() bool {
	return n.niece[0] != nil && n.niece[1] != nil
}

// deadEnd returns true if both neices are nill
// could also return true if n itself is nil! (maybe a bad idea?)
func (n *polNode) deadEnd() bool {
	// if n == nil {
	// 	fmt.Printf("nil deadend\n")
	// 	return true
	// }
	return n.niece[0] == nil && n.niece[1] == nil
}

// chop turns a node into a deadEnd
func (n *polNode) chop() {
	n.niece[0] = nil
	n.niece[1] = nil
}

// prune prunes deadend children.
// don't prune at the bottom; use leaf prune instead at height 1
func (n *polNode) prune() {
	if n.niece[0].deadEnd() {
		n.niece[0] = nil
	}
	if n.niece[1].deadEnd() {
		n.niece[1] = nil
	}
}

// leafPrune is the prune method for leaves.  You don't want to chop off a leaf
// just because it's not memorable; it might be there because its sibling is
// memorable.  Call this at height 1 (not 0)
func (n *polNode) leafPrune() {
	if n.niece[0] != nil && n.niece[1] != nil &&
		n.niece[0].deadEnd() && n.niece[1].deadEnd() {
		n.chop()
	}
}

func (p *Pollard) height() uint8 { return treeHeight(p.numLeaves) }

// TopHashesReverse is ugly and returns the top hashes in reverse order
// ... which is the order full forest is using until I can refactor that code
// to make it big to small order
func (p *Pollard) topHashesReverse() []Hash {
	rHashes := make([]Hash, len(p.tops))
	for i, n := range p.tops {
		rHashes[len(rHashes)-(1+i)] = n.data
	}
	return rHashes
}
