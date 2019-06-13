package utreexo

import "fmt"

// Pollard is the sparse representation of the utreexo forest, using
// binary tree pointers instead of a hash map.

// I generally avoid recursion as much as I can, using regular for loops and
// ranges instead.  That might start looking pretty contrived here, but
// I'm still going to try it.

type Pollard struct {
	numLeaves uint64 // number of leaves in the pollard forest

	tops []*polNode // slice of the tree tops, which are polNodes.
	// tops are in big to small order
	// BUT THEY'RE WEIRD!  The left / right children are actual children,
	// not neices as they are in every lower level.

	hashesEver, rememberEver, overWire uint64

	// the forget slice is all the leaves to forget.  It gets populated when you
	// ingest a block proof and only refers to leaves which will are part of the
	// proof but don't need to stay around after.
	forget []uint64

	//	Lookahead int32  // remember leafs below this TTL
	//	Minleaves uint64 // remember everything below this leaf count

	// rememberLeaf refers to the 1-tree.  If de-treed, is it worth remembering?
	rememberLeaf bool

	// the memorabilityNode isn't actually a node; it's a flag that a leaf is
	// memorable; if the leaf has a pointer to this it's memorable; if it doesn't
	// it's forgettable and exists only as proof for its adjacent node.
	// leaves won't count as "deadEnds" if they point to this
	memorabilityNode polNode
}

// PolNode is a node in the pollard forest
type polNode struct {
	data  Hash
	niece [2]*polNode
	//	flags byte
}

// auntOp returns the hash of a nodes neices. crashes if you call on nil neices.
func (n *polNode) auntOp() Hash {
	return Parent(n.niece[0].data, n.niece[1].data)
}

// deadEnd returns true if both neices are nill
func (n *polNode) deadEnd() bool {
	return n.niece[0] == nil && n.niece[1] == nil
}

// chop turns a node into a deadEnd
func (n *polNode) chop() {
	n.niece[0] = nil
	n.niece[1] = nil
}

func (p *Pollard) height() uint8 { return treeHeight(p.numLeaves) }

// TopHashesReverse is ugly and returns the top hashes in reverse order
// ... which is the order full forest is using until I can refactor that code
// to make it big to small order
func (p *Pollard) TopHashesReverse() []Hash {
	rHashes := make([]Hash, len(p.tops))
	for i, n := range p.tops {
		rHashes[len(rHashes)-(1+i)] = n.data
	}
	return rHashes
}

// Modify is the main function that deletes then adds elements to the accumulator
func (p *Pollard) Modify(adds []LeafTXO, dels []uint64) error {
	err := p.rem(dels)
	if err != nil {
		return err
	}

	err = p.Add(adds)
	if err != nil {
		return err
	}

	return nil
}

func (p *Pollard) Stats() string {
	s := fmt.Sprintf("pol nl %d tops %d he %d re %d ow %d \n",
		p.numLeaves, len(p.tops), p.hashesEver, p.rememberEver, p.overWire)
	return s
}

func (p *Pollard) NumTops() uint8 {
	return uint8(len(p.tops))
}

// Add a leaf to a pollard.  Not as simple!
func (p *Pollard) Add(adds []LeafTXO) error {

	// General algo goes:
	// 1 make a new node & assign data (no neices; at bottom)
	// 2 if this node is on a height where there's already a top,
	// then swap neices with that top, hash the two datas, and build a new
	// node 1 higher pointing to them.
	// goto 2.

	// this does everything 1 at a time, with hashing also mixed in, so that's
	// pretty sub-optimal, but we're not doing multi-thread yet

	for _, a := range adds {

		//		if p.numLeaves < p.Minleaves ||
		//			(add.Duration < p.Lookahead && add.Duration > 0) {
		//			remember = true
		//			p.rememberEver++
		//		}
		if a.Remember {
			p.rememberEver++
		}

		err := p.addOne(a.Hash, a.Remember)
		if err != nil {
			return err
		}
	}
	//	fmt.Printf("added %d, nl %d tops %d\n", len(adds), p.numLeaves, len(p.tops))
	return nil
}

/*
Algo explanation with catchy terms: grab, swap, hash, new, pop
we're iterating through the tops of the pollard.  Tops correspond with 1-bits
in numLeaves.  As soon as we hit a 0 (no top), we're done.

grab: Grab the lowest top.
pop: pop off the lowest top.
swap: swap the neices of the node we grabbed and our new node
hash: calculate the hashes of the old top and new node
new: create a new parent node, with the hash as data, and the old top / prev new node
as neices (not neices though, children)

It's pretty dense: very little code but a bunch going on.

Not that `p.tops = p.tops[:len(p.tops)-1]` would be a memory leak (I guess?)
but that leftTop is still being pointed to anyway do it's OK.

*/

// add a single leaf to a pollard
func (p *Pollard) addOne(add Hash, remember bool) error {
	// basic idea: you're going to start at the LSB and move left;
	// the first 0 you find you're going to turn into a 1.

	// make the new leaf & populate it with the actual data you're trying to add
	n := new(polNode)
	n.data = add
	if remember {
		// flag this leaf as memorable via it's left pointer
		n.niece[0] = &p.memorabilityNode
	}
	// if add is forgetable, forget all the new nodes made
	var h uint8
	// loop until we find a zero; destroy tops until you make one
	for ; (p.numLeaves>>h)&1 == 1; h++ {
		// grab, pop, swap, hash, new
		leftTop := p.tops[len(p.tops)-1]                          // grab
		p.tops = p.tops[:len(p.tops)-1]                           // pop
		leftTop.niece, n.niece = n.niece, leftTop.niece           // swap
		nHash := Parent(leftTop.data, n.data)                     // hash
		n = &polNode{data: nHash, niece: [2]*polNode{leftTop, n}} // new
		p.hashesEver++

		// at height 0, remembering is determined by flags
		if h == 0 {
			if !remember {
				n.niece[1] = nil
			}
			if !p.rememberLeaf {
				n.niece[0] = nil
			}
		} else { // higher up, it's determined by populated nieces
			if n.niece[0].deadEnd() {
				n.niece[0] = nil
			}
			if n.niece[1].deadEnd() {
				n.niece[1] = nil
			}
		}
	}

	if h == 0 {
		p.rememberLeaf = remember
	}

	// the new tops are all the 1 bits above where we got to, and nothing below where
	// we got to.  We've already deleted all the lower tops, so append the new
	// one we just made onto the end.

	p.tops = append(p.tops, n)

	p.numLeaves++
	return nil
}

// TODO for rem:
// turn the dirtymap into a slice. I think it's always ordered so easy
// get rid of dirtymap and rehash entirely?  Not sure if we can
// if not then make it so only rehash hashes, and movenode doesn't.  same with
// pruning?  seems that rehash and movenode do a lot of the same things.

// rem deletes stuff from the pollard.  The hard part.
func (p *Pollard) rem(dels []uint64) error {
	if len(dels) == 0 {
		return nil // that was quick
	}

	ph := p.height()
	nextNumLeaves := p.numLeaves - uint64(len(dels))
	overlap := p.numLeaves & nextNumLeaves

	// remove tops and add empty tops based just on popcount
	nexTops := make([]*polNode, PopCount(nextNumLeaves))
	// keeping track of these separately is annoying.  I'm sure there's a
	// clever bit shifty way to not need to do this.  It doesn't actually
	// take any cpu time or ram though.
	oldTopIdx := len(p.tops) - 1
	nexTopIdx := len(nexTops) - 1

	stash, moves, err := removeTransform(dels, p.numLeaves, ph)
	if err != nil {
		return err
	}

	// TODO how about instead of a map or even a slice of uint64s, you just
	// have a slice of pointers?  And you need to run AuntOp on these pointers
	// if you aren't doing it already from something else.
	nextDirtyMap := make(map[uint64]bool) // whatever use a map for now.

	// can use some kind of queues or something later.

	//	fmt.Printf("p.h %d nl %d rem %d nnl %d stashes %d moves %d\n",
	//		ph, p.numLeaves, len(dels), nextNumLeaves, len(rawStash), len(rawMoves))

	for h := uint8(0); h < ph; h++ {

		//		if verbose {
		// fmt.Printf("pol rem row %d\n", h)
		//		}
		curDirtyMap := nextDirtyMap
		nextDirtyMap = make(map[uint64]bool)

		// copy the top over directly if there's a bit overlap
		// fmt.Printf("h %d topIdx %d overlap %b\n", h, nexTopIdx, overlap)
		if (1<<h)&overlap != 0 {
			// fmt.Printf("topidx %d nexTops %d ptops %d\n",
			// topIdx, len(nexTops), len(p.tops))
			nexTops[nexTopIdx] = p.tops[oldTopIdx]
		}

		// go through moves for this height
		for len(moves) > 0 && detectHeight(moves[0].to, ph) == h {
			if len(p.tops) == 0 || p.tops[0] == nil {
				return fmt.Errorf("no tops...")
			}
			//			fmt.Printf("mv %d -> %d\n", rawMoves[0].from, rawMoves[0].to)
			dirt, err := p.moveNode(moves[0], curDirtyMap)
			if err != nil {
				return err
			}
			// the dirt returned by moveNode is always a parent so can never be 0
			if dirt != 0 && inForest(dirt, p.numLeaves) {
				nextDirtyMap[dirt] = true
			}
			moves = moves[1:]
		}

		// then the stash on this height.  (There can be only 1)
		for len(stash) > 0 &&
			detectHeight(stash[0].to, ph) == h {
			// populate top; stashes always become tops
			//			fmt.Printf("stash %d -> %d\n", rawStash[0].from, rawStash[0].to)
			pr, sibs, err := p.DescendToPos(stash[0].from)
			if err != nil {
				return fmt.Errorf("rem stash %s", err.Error())
			}

			if sibs[0] == nil {
				return fmt.Errorf("got nil sib[0] stashing")
			}
			// make new top if sibling nieces are known
			// otherwise need to delete the neices (same thing really,
			// just doesn't crash)
			if pr[0] != nil {
				sibs[0].niece = pr[0].niece
			} else {
				sibs[0].chop()
			}

			// if stashing to height 0, it may be forgettable,
			// which we need to track (& pop the forget slice) (which should
			// be len 1)
			if len(p.forget) > 1 {
				return fmt.Errorf("more than 1 forget at stash phase")
			}
			if h == 0 {
				if len(p.forget) == 1 {
					// the remaining forget should be the from position.
					// If not, something's wrong
					if p.forget[0] != stash[0].from {
						return fmt.Errorf("last forget is %d but stash from %d",
							p.forget[0], stash[0].from)
					}
					// set forgettable leaf and pop (clear) forget slice
					p.rememberLeaf = false
					p.forget = []uint64{}
				} else if stash[0].from < stash[0].to {
					// if a stash is moving left, that means it already was a stash
					// and thus memorability shouldn't be changed
					p.rememberLeaf = true
				}
			}

			nexTops[nexTopIdx] = sibs[0]
			stash = stash[1:]
		}

		// if we're not at the top, and there's curDirtyMap left, hash
		if h < ph-1 {
			for pos, _ := range curDirtyMap {
				err := p.reHashOne(pos)
				if err != nil {
					return fmt.Errorf("rem rehash %s", err.Error())
				}

				parPos := up1(pos, ph)
				if inForest(parPos, p.numLeaves) {
					//  fmt.Printf("adding dirt %d\n", parPos)
					nextDirtyMap[parPos] = true
				} else {
					//	 fmt.Printf("couldn't add dirt %d\n", parPos)
				}
			}
		}

		// if there's a 1 in the nextNum, decrement top number
		if (1<<h)&nextNumLeaves != 0 {
			nexTopIdx--
		}
		if (1<<h)&p.numLeaves != 0 {
			oldTopIdx--
		}
	}

	p.numLeaves = nextNumLeaves
	p.tops = nexTops

	return nil
}

// swap moves a node from one place to another.  Note that it leaves the
// node in the "from" place to be dealt with some other way.
// Also it hashes new parents so the hashes & pointers are consistent.
func (p *Pollard) moveNode(m move, cdm map[uint64]bool) (uint64, error) {

	prfrom, sibfrom, err := p.DescendToPos(m.from)
	if err != nil {
		return 0, fmt.Errorf("from %s", err.Error())
	}

	prto, sibto, err := p.DescendToPos(m.to)
	if err != nil {
		return 0, fmt.Errorf("to %s", err.Error())
	}

	tgtLR := m.to & 1
	//	fmt.Printf("movenode prfrom %d prto %d\n", len(prfrom), len(prto))

	// more generally -- ANYTHING that is a deadEnd should be deleted
	// forget slice should only persist between ingest and rem;
	// rem can handle deletions / forgetting, as it basically already does.

	// evap is if the move target (m.to) should evaporate after moving
	// evapSib is if the target's sibling (m.to^1) should evaporate after the move
	var evapTgt, evapSib bool

	// sibling needs to be forgotten if:
	// it's at the front of the forget slice (in which case pop)
	if len(p.forget) > 0 && m.to^1 == p.forget[0] {
		evapSib = true
		p.forget = p.forget[1:]
		// pop, important to do this before the evapTgt check
	}

	// target needs to be forgotten if:
	// the source (m.from) is the top of the forget slice, in which case pop
	if len(p.forget) > 0 && m.from == p.forget[0] {
		evapTgt = true
		p.forget = p.forget[1:]
	}

	// target should also be forgotten if:
	// it was a top, has no nieces, and wasn't a memorable 0-top
	// (pretty sure this is exclusive of the prior condition but whatever)
	if sibfrom[0] == prfrom[0] && // it's a top
		sibfrom[0].deadEnd() && // it has no nieces
		!(m.from < p.numLeaves && p.rememberLeaf) { // it's not a memorable 0top
		evapTgt = true
	}

	// create & link sibto if it doesn't exist.
	// don't need to for prto as it must already exist... (I think?)
	if sibto[0] == nil {
		sibto[0] = new(polNode)
		//		fmt.Printf("mv make dest node at %d\n", to)
		if prto[1] != nil {
			prto[1].niece[tgtLR] = sibto[0]
		}
	}

	// this works even if moving from a top, because sibfrom[0] and prfrom[0]
	// will be the same
	// gotta move the data, but move nieces if you can.  If you can't move the
	// nieces you have to delete the destination nieces!
	//	fmt.Printf("move %04x over %04x prfromnil %v\n",
	//		sibfrom[0].data[:4], sibto[0].data[:4], prfrom[0] == nil)
	sibto[0].data = sibfrom[0].data
	if prfrom[0] != nil {
		prto[0].niece = prfrom[0].niece
	} else {
		prto[0].chop() // need this
	}

	// now hash (if there's something above) (Which there always will be...?)
	if len(prto) > 1 { // true unless moving to a top? in which case just return?
		if sibto[1] == nil { // create & link if it doesn't exist
			// fmt.Printf("mv make parent node at %d\n", up1(to, p.height()))
			sibto[1] = new(polNode)
			if prto[2] != nil {
				prto[2].niece[(m.to>>1)&1] = sibto[1]
			}
		}
		p.hashesEver++
		sibto[1].data = prto[1].auntOp()
		//		fmt.Printf("compute %04x at %d evap tgt %v sib %v\n",
		//			sibto[1].data[:4], up1(m.to, p.height()), evapTgt, evapSib)
		// after auntopping, delete nodes.
		if m.to < p.numLeaves {
			// at height 0, we can cut the pointers to tgt and sib based on the earlier bools
			if evapTgt {
				prto[1].niece[tgtLR] = nil
			}
			if evapSib {
				prto[1].niece[tgtLR^1] = nil
			}
		} else {
			//   on upper heights, a node has no nieces, it can be cut.
			if sibto[0].deadEnd() {
				prto[1].niece[tgtLR] = nil
			}
			if prto[0].deadEnd() {
				prto[1].niece[tgtLR^1] = nil
			}
		}
	}

	ph := p.height()
	//  just hashed to's parent, so delete that from cdm
	delete(cdm, up1(m.from, ph))
	delete(cdm, up1(m.to, ph))
	parPos := upMany(m.to, 2, ph)

	return parPos, nil
}

// the Hash & trim function called by rem().  Not currently called on leaves
func (p *Pollard) reHashOne(pos uint64) error {

	pr, sib, err := p.DescendToPos(pos)
	if err != nil {
		return err
	}
	//	fmt.Printf("reHashOne %d pr %d sib %d pr[0] %v\n",
	//		pos, len(pr), len(sib), pr[0].niece)
	if sib[0] == nil {
		sib[0] = new(polNode)
		pr[1].niece[pos&1] = sib[0]
		//		return fmt.Errorf("sib[0] nil")
	}
	sib[0].data = pr[0].auntOp()
	//	fmt.Printf("rehash %04x\n", sib[0].data[:4])
	p.hashesEver++
	// check for dead ends to be cut.  (reHashOne only occurs on upper levels)
	if pr[0].niece[0].deadEnd() {
		pr[0].niece[0] = nil
	}
	if pr[0].niece[1].deadEnd() {
		pr[0].niece[1] = nil
	}
	return nil
}

// DescendToPos returns the path to the target node, as well as the sibling
// path.  Retruns paths in bottom-to-top order (backwards)
func (p *Pollard) DescendToPos(pos uint64) ([]*polNode, []*polNode, error) {
	// interate to descend.  It's like the leafnum, xored with ...1111110
	// so flip every bit except the last one.
	// example: I want leaf 12.  That's 1100.  xor to get 0010.
	// descent 0, 0, 1, 0 (left, left, right, left) to get to 12 from 30.
	// need to figure out offsets for smaller trees.

	//	nh := detectHeight(pos, p.height())

	if !inForest(pos, p.numLeaves) {
		//	if (nh == 0 && pos >= p.numLeaves) || pos >= (p.numLeaves*2)-1 {
		// need better check for "there isn't a node there"
		return nil, nil,
			fmt.Errorf("OOB: descend to %d but only %d leaves", pos, p.numLeaves)
	}

	// first find which tree we're in
	tNum, bits, branchLen := detectOffset(pos, p.numLeaves)
	//	fmt.Printf("DO pos %d top %d bits %d branlen %d\n", pos, tNum, bits, branchLen)
	n := p.tops[tNum]
	if n == nil || branchLen > 64 {
		return nil, nil, fmt.Errorf("dtp top %d is nil", tNum)
	}

	bits = ^bits // just flip all the bits...
	proofs := make([]*polNode, branchLen+1)
	sibs := make([]*polNode, branchLen+1)
	// at the top of the branch, the proof and sib are the same
	proofs[branchLen], sibs[branchLen] = n, n
	for h := branchLen - 1; h < 64; h-- {
		lr := (bits >> h) & 1

		sib := n.niece[lr^1]
		n = n.niece[lr]

		if n == nil && h != 0 {
			return nil, nil, fmt.Errorf(
				"descend pos %d nil neice at height %d", pos, h)
		}

		if n != nil {
			// fmt.Printf("target %d h %d d %04x\n", pos, h, n.data[:4])
		}

		proofs[h], sibs[h] = n, sib

	}
	//	fmt.Printf("\n")
	return proofs, sibs, nil
}

func (p *Pollard) ToStringTops() string {
	s := "tops: "

	for i, t := range p.tops {
		s += fmt.Sprintf("%d:%04x ", i, t.data[:4])
	}
	s += fmt.Sprintf("\n")
	return s
}

func (p *Pollard) ToStringLeaves() string {
	var s string
	// fmt.Printf("h %d nl %d tops %d\n", p.height(), p.numLeaves, len(p.tops))
	for i := uint64(0); i < p.numLeaves; i++ {
		_, sibs, err := p.DescendToPos(i)
		if err != nil {
			fmt.Printf("leaf %d %s\n", i, err.Error())
			continue
			//			panic(err.Error())
			//			return err.Error()
		}
		if sibs[0] != nil {
			s += fmt.Sprintf("%d: %04x (path %d)\n", i, sibs[0].data[:4], len(sibs))
		}
	}
	return s
}

// toFull takes a pollard and converts to a forest.
// For debugging and seeing what pollard is doing since there's already
// a good toString method for  forest.
func (p *Pollard) toFull() (*Forest, error) {
	ff := NewForest()
	ff.height = p.height()
	ff.numLeaves = p.numLeaves
	ff.forest = make([]Hash, 2<<ff.height)

	//	for topIdx, top := range p.tops {
	//	}
	for i := uint64(0); i < (2<<ff.height)-1; i++ {
		_, sib, err := p.DescendToPos(i)
		if err != nil {
			//	fmt.Printf("can't get pos %d: %s\n", i, err.Error())
			continue
			//			return nil, err
		}
		if sib[0] != nil {
			ff.forest[i] = sib[0].data
			//	fmt.Printf("wrote leaf pos %d %04x\n", i, sib[0].data[:4])
		}

	}

	return ff, nil
}
