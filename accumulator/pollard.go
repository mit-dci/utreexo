package accumulator

import (
	"fmt"
)

const (
	ErrorNotEnoughTrees uint32 = iota
	ErrorNoPollardNode
)

var ErrorStrings = map[uint32]error{
	ErrorNotEnoughTrees: fmt.Errorf("ErrorNotEnoughTrees"),
	ErrorNoPollardNode:  fmt.Errorf("ErrorNoPollardNode"),
}

// Pollard is the sparse representation of the utreexo forest, represented as
// a collection of binary trees.
type Pollard struct {
	// number of leaves in the pollard forest
	numLeaves uint64

	// roots are the slice of the tree roots ordered in big to small
	// (right to left of the forest).
	//
	// NOTE: Since roots don't have nieces, they point to children.
	// In the below tree, 06 is the root and it points to its children,
	// 04 and 05. However, 04 points to 02 and 03; 05 points to 00 and 01.
	// 04 and 05 are pointing to their nieces.
	// The leaves are also different in that they don't point to anything
	// (as of now, the leaves point to themselves if it's to be cached).
	//
	// 06
	// |-------\
	// 04      05
	// |---\   |---\
	// 00  01  02  03
	roots []*polNode

	// Lookahead is the threshold that sets which leaves should be cached.
	// TODO not currently implemented yet.
	Lookahead int32

	// positionMap is maps hashes to positions.
	// It is only used for fullPollard.
	positionMap map[MiniHash]uint64

	// these three are for keeping statistics.
	// hashesEver is all the hashes that have ever been performed.
	// rememberEver is all the nodes that have ever been cached.
	// overWire is all the leaves that have been received over the network
	hashesEver, rememberEver, overWire uint64
}

// Modify deletes then adds elements to the accumulator.
func (p *Pollard) Modify(adds []Leaf, delsUn []uint64) error {
	dels := make([]uint64, len(delsUn))
	copy(dels, delsUn)
	sortUint64s(dels)

	err := p.rem2(dels)
	if err != nil {
		return err
	}

	err = p.add(adds)
	if err != nil {
		return err
	}

	return nil
}

// Stats returns the current pollard statistics as a string.
func (p *Pollard) Stats() string {
	s := fmt.Sprintf("pol nl %d roots %d he %d re %d ow %d \n",
		p.numLeaves, len(p.roots), p.hashesEver, p.rememberEver, p.overWire)
	return s
}

// ReconstructStats returns numleaves and row so that batch proofs can be
// reconstructed and hashes can be matches.
//
// TODO Replace this with proofs that do not include the things being proven, and
// take the proved leaves as a separate argument
func (p *Pollard) ReconstructStats() (uint64, uint8) {
	return p.numLeaves, p.rows()
}

// add adds all the given adds to the pollard.
func (p *Pollard) add(adds []Leaf) error {

	// General algo goes:
	// 1 make a new node & assign data (no nieces; at bottom)
	// 2 if this node is on a row where there's already a root,
	// then swap nieces with that root, hash the two datas, and build a new
	// node 1 higher pointing to them.
	// goto 2.

	// this does everything 1 at a time, with hashing also mixed in, so that's
	// pretty sub-optimal, but we're not doing multi-thread yet

	for _, a := range adds {
		if a.Remember {
			p.rememberEver++
		}

		err := p.addOne(a.Hash, a.Remember)
		if err != nil {
			return err
		}
	}

	return nil
}

// addOne adds a single leaf to a pollard
func (p *Pollard) addOne(add Hash, remember bool) error {
	// Basic idea: we're iterating through the roots of the pollard and roots correspond with 1-bits.
	// We're going to start at the LSB and move left until we hit a 0 and turn it into a 1.
	//
	// The algorithm can be explained with catchy terms: grab, swap, hash, new, pop.
	// grab: Grab the current lowest root.
	// pop: pop off the current lowest root.
	// swap: swap the nieces of the node we grabbed and our new node.
	// hash: calculate the hashes of the old root and new node.
	// new: create a new parent node, with the hash as data, and the old root / prev new node
	// as nieces (not nieces though, children).
	//
	// For example a tree of five leaves would look like this. We can tell where all the roots are by looking at the binary representation
	// of 5: 101.
	//
	// 12
	// |-------\
	// em      em
	// |---\   |---\
	// em  em  em  em  04
	//
	// The resulting tree would have 6 leaves. The binary representation is now 110 and
	// the tree would look like so:
	//
	// 12
	// |-------\
	// em      em      10
	// |---\   |---\   |---\
	// em  em  em  em  em  em

	n := new(polNode)
	n.data = add
	if remember || p.positionMap != nil {
		// flag this leaf as memorable via it's left pointer
		n.niece[0] = n // points to itself (mind blown)
	}

	if p.positionMap != nil {
		p.positionMap[add.Mini()] = p.numLeaves
	}

	fmt.Printf("Adding hash: %x remember:%t\n",
		add.Mini(), remember)

	// if add is forgetable, forget all the new nodes made
	// loop until we find a zero; destroy roots until you make one
	var h uint8
	for ; (p.numLeaves>>h)&1 == 1; h++ {
		// grab, pop, swap, hash, new
		leftRoot := p.roots[len(p.roots)-1] // grab
		p.roots = p.roots[:len(p.roots)-1]  // pop
		// If we're not at bottom, just swap nieces
		if h != 0 {
			if leftRoot.niece[0] == leftRoot {
				panic("")
			}
			if n.niece[0] == n {
				panic("")
			}
			leftRoot.niece, n.niece = n.niece, leftRoot.niece // swap
		} else {
			// If we're at bottom (aka row 0), check if the leaf is
			// supposed to be remembered.
			if remember {
				// Also remember the leftRoot (sibling) if this leaf is to be
				// remembered. The sibling is needed as a proof for this leaf.
				leftRoot.niece[0] = leftRoot
			} else {
				// Wether is polNode is supposed to be remembered is denoted by:
				// polNode.niece[0] = polNode.
				//
				// If the leftRoot is supposed to be remembered, we also need
				// to remember this leaf regardless of whether Remember is set
				// to true or not.
				if leftRoot.niece[0] == leftRoot {
					n.niece[0] = n
				}
			}
		}
		nHash := parentHash(leftRoot.data, n.data)                 // hash
		n = &polNode{data: nHash, niece: [2]*polNode{leftRoot, n}} // new
		p.hashesEver++

		n.prune()
	}

	// the new roots are all the 1 bits above where we got to, and nothing below where
	// we got to.  We've already deleted all the lower roots, so append the new
	// one we just made onto the end.

	p.roots = append(p.roots, n)
	p.numLeaves++
	return nil
}

// rem2 deletes the passed in dels from the pollard
func (p *Pollard) rem2(dels []uint64) error {
	// Hash and swap.  "grabPos" in rowdirt / hashdirt is inefficient because you
	// descend to the place you already just decended to perform swapNodes.

	// rem2 outline:
	// perform swaps & hash, then select new roots.

	// swap & hash is row based.  Swaps on row 0 cause hashing on row 1.
	// So the sequence is: Swap row 0, hash row 1, swap row 1, hash row 2,
	// swap row 2... etc.
	// The tricky part is that we have a big for loop with h being the current
	// row.  When h=0 and we're swapping things on the bottom, we can hash
	// things at row 1 (h+1).  Before we start swapping row 1, we need to be sure
	// that all hashing for row 1 has finished.
	if len(dels) == 0 {
		return nil // that was quick
	}
	ph := p.rows() // rows of pollard
	nextNumLeaves := p.numLeaves - uint64(len(dels))

	if p.positionMap != nil { // if fulpol, remove hashes from posMap
		for _, delpos := range dels {
			delete(p.positionMap, p.read(delpos).Mini())
		}
	}

	// get all the swaps, then apply them all
	swapRows := remTrans2(dels, p.numLeaves, ph)

	var hashDirt, nextHashDirt []uint64
	var prevHash uint64
	var err error
	// swap & hash all the nodes
	for h := uint8(0); h < ph; h++ {
		var hnslice []*hashableNode
		hashDirt = dedupeSwapDirt(hashDirt, swapRows[h])
		for len(swapRows[h]) != 0 || len(hashDirt) != 0 {
			var hn *hashableNode
			var collapse bool
			// check if doing dirt. if not dirt, swap.
			// (maybe a little clever here...)
			if len(swapRows[h]) == 0 ||
				len(hashDirt) != 0 && hashDirt[0] > swapRows[h][0].to {
				// re-descending here which isn't great
				hn, err = p.hnFromPos(hashDirt[0])
				if err != nil {
					return err
				}
				hashDirt = hashDirt[1:]
			} else { // swapping
				if swapRows[h][0].from == swapRows[h][0].to {
					// TODO should get rid of these upstream
					// panic("got non-moving swap")
					swapRows[h] = swapRows[h][1:]
					continue
				}
				hn, err = p.swapNodes(swapRows[h][0], h)
				if err != nil {
					return err
				}
				collapse = swapRows[h][0].collapse
				swapRows[h] = swapRows[h][1:]
			}
			if hn == nil ||
				hn.position == prevHash || collapse {
				// TODO: there are probably more conditions in which a hn could be skipped.
				continue
			}

			hnslice = append(hnslice, hn)
			prevHash = hn.position
			if len(nextHashDirt) == 0 ||
				(nextHashDirt[len(nextHashDirt)-1] != hn.position) {
				// skip if already on end of slice. redundant?
				nextHashDirt = append(nextHashDirt, hn.position)
			}
		}
		hashDirt = nextHashDirt
		nextHashDirt = []uint64{}
		// do all the hashes at once at the end
		for _, hn := range hnslice {
			// skip hashes we can't compute
			if hn.sib.niece[0] == nil || hn.sib.niece[1] == nil ||
				hn.sib.niece[0].data == empty || hn.sib.niece[1].data == empty {
				// TODO when is hn nil?  is this OK?
				// it'd be better to avoid this and not create hns that aren't
				// supposed to exist.
				continue
			}
			hn.dest.data = hn.sib.auntOp()
			hn.sib.prune()
		}
	}

	positionList := NewPositionList()
	defer positionList.Free()

	// set new roots
	getRootsForwards(nextNumLeaves, ph, &positionList.list)
	nextRoots := make([]*polNode, len(positionList.list))
	for i, _ := range nextRoots {
		rootPos := len(positionList.list) - (i + 1)
		nt, ntsib, _, err := p.grabPos(positionList.list[rootPos])
		if err != nil {
			return err
		}
		if nt == nil {
			return fmt.Errorf("want root %d at %d but nil", i, positionList.list[i])
		}
		if ntsib == nil {
			// when turning a node into a root, it's "nieces" are really children,
			// so should become it's sibling's nieces.
			nt.chop()
		} else {
			nt.niece = ntsib.niece
		}
		nextRoots[i] = nt
	}
	p.numLeaves = nextNumLeaves
	reversePolNodeSlice(nextRoots)
	p.roots = nextRoots
	return nil
}

func (p *Pollard) hnFromPos(pos uint64) (*hashableNode, error) {
	if !inForest(pos, p.numLeaves, p.rows()) {
		return nil, nil
	}
	_, _, hn, err := p.grabPos(pos)
	if err != nil {
		return nil, err
	}
	hn.position = parent(pos, p.rows())
	return hn, nil
}

// swapNodes swaps the nodes at positions a and b.
// returns a hashable node with b, bsib, and bpar
func (p *Pollard) swapNodes(s arrow, row uint8) (*hashableNode, error) {
	// First operate on the position map for the fullPollard types
	if p.positionMap != nil {
		a := childMany(s.from, row, p.rows())
		b := childMany(s.to, row, p.rows())
		run := uint64(1 << row)
		// happens before the actual swap, so swapping a and b
		for i := uint64(0); i < run; i++ {
			p.positionMap[p.read(a+i).Mini()] = b + i
			p.positionMap[p.read(b+i).Mini()] = a + i
		}
	}

	// currently swaps the "values" instead of changing what parents point
	// to.  Seems easier to reason about but maybe slower?  But probably
	// doesn't matter that much because it's changing 8 bytes vs 30-something

	// TODO could be improved by getting the highest common ancestor
	// and then splitting instead of doing 2 full descents

	a, asib, _, err := p.grabPos(s.from)
	if err != nil {
		return nil, err
	}
	b, bsib, bhn, err := p.grabPos(s.to)
	if err != nil {
		return nil, err
	}
	if asib == nil || bsib == nil {
		return nil, fmt.Errorf("swapNodes %d %d sibling not found", s.from, s.to)
	}
	if a == nil || b == nil {
		return nil, fmt.Errorf("swapNodes %d %d node not found", s.from, s.to)
	}

	bhn.position = parent(s.to, p.rows())
	// do the actual swap here
	if row == 0 {
		err = polSwap(a, asib, b, bsib)
		if err != nil {
			return nil, err
		}

		if a.niece[0] == a {
			fmt.Println("a.niece[0] == a")
		}
		if asib.niece[0] == asib {
			fmt.Println("asib.niece[0] == asib")
		}
		if b.niece[0] == b {
			fmt.Println("b.niece[0] == b")
		}
		if bsib.niece[0] == bsib {
			fmt.Println("bsib.niece[0] == bsib")
		}
	} else {
		err = polSwap(a, asib, b, bsib)
		if err != nil {
			return nil, err
		}
	}
	if bhn.sib.niece[0].data == empty || bhn.sib.niece[1].data == empty {
		bhn = nil // we can't perform this hash as we don't know the children
	}

	return bhn, nil
}

// readPos returns a pointer to the node at the requested position. readPos does not
// mutate the Pollard in any way.
func (p *Pollard) readPos(pos uint64) (
	n, nsib *polNode, hn *hashableNode, err error) {
	// Grab the tree that the position is at
	tree, branchLen, bits := detectOffset(pos, p.numLeaves)
	if tree >= uint8(len(p.roots)) {
		err = ErrorStrings[ErrorNotEnoughTrees]
		return
	}

	n, nsib = p.roots[tree], p.roots[tree]

	if branchLen == 0 {
		return
	}

	for h := branchLen - 1; h != 0; h-- { // go through branch
		lr := uint8(bits>>h) & 1
		// grab the sibling of lr
		lrSib := lr ^ 1

		n, nsib = n.niece[lr], n.niece[lrSib]
		if n == nil {
			return nil, nil, nil, err
		}
	}

	lr := uint8(bits) & 1
	// grab the sibling of lr
	lrSib := lr ^ 1

	n, nsib = n.niece[lrSib], n.niece[lr]
	return // only happens when returning a root
}

// grabPos takes the given position and returns a pointer to the polNode at the position,
// its sibling, and a hashableNode at the parent position.
// Returns an error if it can't get it.
//
// NOTE grabPos will attach an empty polNode while descending down the tree for all
// empty siblings. This mutates the Pollard.
// NOTE errors are not exhaustive; could return garbage without an error
func (p *Pollard) grabPos(
	pos uint64) (n, nsib *polNode, hn *hashableNode, err error) {
	// Grab the tree that the position is at
	tree, branchLen, bits := detectOffset(pos, p.numLeaves)
	if tree >= uint8(len(p.roots)) {
		err = ErrorStrings[ErrorNotEnoughTrees]
		return
	}
	n, nsib = p.roots[tree], p.roots[tree]

	hn = &hashableNode{dest: n, sib: nsib}

	if branchLen == 0 {
		return
	}

	for h := branchLen - 1; h != 0; h-- { // go through branch
		lr := uint8(bits>>h) & 1
		// grab the sibling of lr
		lrSib := lr ^ 1

		// if a sib doesn't exist, need to create it and hook it in
		if n.niece[lrSib] == nil {
			n.niece[lrSib] = &polNode{}
		}
		n, nsib = n.niece[lr], n.niece[lrSib]
		if n == nil {
			// if a node doesn't exist, crash
			// no niece in this case
			// TODO error message could be better
			err = ErrorStrings[ErrorNoPollardNode]
			return
		}
	}

	lr := uint8(bits) & 1
	// grab the sibling of lr
	lrSib := lr ^ 1

	hn.dest = nsib // this is kind of confusing eh?
	hn.sib = n     // but yeah, switch siblingness
	n, nsib = n.niece[lrSib], n.niece[lr]
	return // only happens when returning a root
}

// toFull takes a pollard and converts to a forest.
// For debugging and seeing what pollard is doing since there's already
// a good toString method for  forest.
func (p *Pollard) toFull() (*Forest, error) {
	ff := NewForest(nil, false, "", 0)
	ff.rows = p.rows()
	ff.numLeaves = p.numLeaves
	ff.data = new(ramForestData)
	ff.data.resize((2 << ff.rows) - 1)
	if p.numLeaves == 0 {
		return ff, nil
	}

	// very naive loop looping outside the edge of the tree
	for i := uint64(0); i < (2<<ff.rows)-1; i++ {
		// check if the leaf is within the tree
		if !inForest(i, ff.numLeaves, ff.rows) {
			continue
		}
		n, _, _, err := p.readPos(i)
		if err != nil {
			return nil, err
		}
		if n != nil {
			ff.data.write(i, n.data)
		}
	}

	return ff, nil
}

// GetRoots returns the hashes of the pollard roots
func (p *Pollard) GetRoots() (h []Hash) {
	// pre-allocate. Shouldn't matter too much because this is only to export the
	// utreexo state
	h = make([]Hash, 0, len(p.roots))

	for _, pn := range p.roots {
		h = append(h, pn.data)
	}
	return
}

// ToString returns a string visualization of the Pollard that can be printed
func (p *Pollard) ToString() string {
	f, err := p.toFull()
	if err != nil {
		return err.Error()
	}
	return f.ToString()
}

// equalToForest checks if the pollard has the same leaves as the forest.
// doesn't check roots and stuff
func (p *Pollard) equalToForest(f *Forest) bool {
	if p.numLeaves != f.numLeaves {
		return false
	}

	for leafpos := uint64(0); leafpos < f.numLeaves; leafpos++ {
		n, _, _, err := p.grabPos(leafpos)
		if err != nil {
			return false
		}
		if n.data != f.data.read(leafpos) {
			fmt.Printf("leaf position %d pol %x != forest %x\n",
				leafpos, n.data[:4], f.data.read(leafpos).Prefix())
			return false
		}
	}
	return true
}

// equalToForestIfThere checks if the pollard has the same leaves as the forest.
// it's OK though for a leaf not to be there; only it can't exist and have
// a different value than one in the forest
func (p *Pollard) equalToForestIfThere(f *Forest) bool {
	if p.numLeaves != f.numLeaves {
		return false
	}

	for leafpos := uint64(0); leafpos < f.numLeaves; leafpos++ {
		n, _, _, err := p.grabPos(leafpos)
		if err != nil || n == nil {
			continue // ignore grabPos errors / nils
		}
		if n.data != f.data.read(leafpos) {
			fmt.Printf("leaf position %d pol %x != forest %x\n",
				leafpos, n.data[:4], f.data.read(leafpos).Prefix())
			return false
		}
	}
	return true
}
