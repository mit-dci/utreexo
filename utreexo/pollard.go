package utreexo

import (
	"fmt"
	"sync"
)

// Modify is the main function that deletes then adds elements to the accumulator
func (p *Pollard) Modify(adds []LeafTXO, dels []uint64) error {
	err := p.rem2(dels)
	if err != nil {
		return err
	}
	// fmt.Printf("pol pre add %s", p.toString())

	err = p.add(adds)
	if err != nil {
		return err
	}

	return nil
}

// Stats :
func (p *Pollard) Stats() string {
	s := fmt.Sprintf("pol nl %d tops %d he %d re %d ow %d \n",
		p.numLeaves, len(p.tops), p.hashesEver, p.rememberEver, p.overWire)
	return s
}

// Add a leaf to a pollard.  Not as simple!
func (p *Pollard) add(adds []LeafTXO) error {

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
	if remember || p.positionMap != nil {
		// flag this leaf as memorable via it's left pointer
		n.niece[0] = n // points to itself (mind blown)
	}

	if p.positionMap != nil {
		p.positionMap[add.Mini()] = p.numLeaves
	}

	// if add is forgetable, forget all the new nodes made
	var h uint8
	// loop until we find a zero; destroy tops until you make one
	for ; (p.numLeaves>>h)&1 == 1; h++ {
		// grab, pop, swap, hash, new
		leftTop := p.tops[len(p.tops)-1]                           // grab
		p.tops = p.tops[:len(p.tops)-1]                            // pop
		leftTop.niece, n.niece = n.niece, leftTop.niece            // swap
		nHash := Parent(leftTop.data, n.data)                      // hash
		n = &polNode{data: nHash, niece: [2]*polNode{&leftTop, n}} // new
		p.hashesEver++

		n.prune()

	}

	// the new tops are all the 1 bits above where we got to, and nothing below where
	// we got to.  We've already deleted all the lower tops, so append the new
	// one we just made onto the end.

	p.tops = append(p.tops, *n)
	p.numLeaves++
	return nil
}

// Hash and swap.  "grabPos" in rowdirt / hashdirt is inefficient because you
// descend to the place you already just decended to perfom swapNodes.

// rem2 outline:
// perform swaps & hash, then select new tops.

// swap & hash is row based.  Swaps on row 0 cause hashing on row 1.
// So the sequence is: Swap row 0, hash row 1, swap row 1, hash row 2,
// swap row 2... etc.
// The tricky part is that we have a big for loop with h being the current
// height.  When h=0 and we're swapping things on the bottom, we can hash
// things at row 1 (h+1).  Before we start swapping row 1, we need to be sure
// that all hashing for row 1 has finished.

// rem2 deletes stuff from the pollard, using remtrans2
func (p *Pollard) rem2(dels []uint64) error {
	if len(dels) == 0 {
		return nil // that was quick
	}
	ph := p.height() // height of pollard
	nextNumLeaves := p.numLeaves - uint64(len(dels))

	if p.positionMap != nil { // if fulpol, remove hashes from posMap
		for _, delpos := range dels {
			delete(p.positionMap, p.read(delpos).Mini())
		}
	}

	// get all the swaps, then apply them all
	swaprows := remTrans2(dels, p.numLeaves, ph)
	wg := new(sync.WaitGroup)
	// fmt.Printf(" @@@@@@ rem2 nl %d ph %d rem %v\n", p.numLeaves, ph, dels)
	var hashDirt, nextHashDirt []uint64
	var prevHash uint64
	var err error
	// fmt.Printf("start rem %s", p.toString())
	// swap & hash all the nodes
	for h := uint8(0); h < ph; h++ {
		var hnslice []*hashableNode
		// fmt.Printf("row %d hd %v nhd %v swaps %v\n", h, hashDirt, nextHashDirt, swaprows[h])
		hashDirt = dedupeSwapDirt(hashDirt, swaprows[h])
		// fmt.Printf("row %d hd %v nhd %v swaps %v\n", h, hashDirt, nextHashDirt, swaprows[h])
		for len(swaprows[h]) != 0 || len(hashDirt) != 0 {
			var hn *hashableNode
			// check if doing dirt. if not dirt, swap.
			// (maybe a little clever here...)
			if len(swaprows[h]) == 0 ||
				len(hashDirt) != 0 && hashDirt[0] > swaprows[h][0].to {
				// re-descending here which isn't great
				// fmt.Printf("hashing from dirt %d\n", hashDirt[0])
				hn, err = p.hnFromPos(hashDirt[0])
				if err != nil {
					return err
				}
				hashDirt = hashDirt[1:]
			} else { // swapping
				// fmt.Printf("swapping %v\n", swaprows[h][0])
				if swaprows[h][0].from == swaprows[h][0].to {
					// TODO should get rid of these upstream
					// panic("got non-moving swap")
					swaprows[h] = swaprows[h][1:]
					continue
				}
				hn, err = p.swapNodes(swaprows[h][0], h)
				if err != nil {
					return err
				}
				swaprows[h] = swaprows[h][1:]
			}
			if hn == nil {
				continue
			}
			if hn.position == prevHash { // we just did this
				// fmt.Printf("just did %d\n", prevHash)
				continue // TODO this doesn't cover eveything
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
		wg.Add(len(hnslice))
		for _, hn := range hnslice {
			// skip hashes we can't compute
			if hn.sib.niece[0] == nil || hn.sib.niece[1] == nil ||
				hn.sib.niece[0].data == empty || hn.sib.niece[1].data == empty {
				// TODO when is hn nil?  is this OK?
				// it'd be better to avoid this and not create hns that aren't
				// supposed to exist.
				fmt.Printf("hn %d nil or uncomputable\n", hn.position)
				wg.Done()
				continue
			}
			// fmt.Printf("giving hasher %d %x %x\n",
			// hn.position, hn.sib.niece[0].data[:4], hn.sib.niece[1].data[:4])
			go hn.run(wg)
		}
		wg.Wait() // wait for all hashing to finish at end of each row
		// fmt.Printf("done with row %d %s\n", h, p.toString())
	}

	// fmt.Printf("pretop %s", p.toString())
	// set new tops
	nextTopPoss, _ := getTopsReverse(nextNumLeaves, ph)
	nexTops := make([]polNode, len(nextTopPoss))
	for i, _ := range nexTops {
		nt, ntsib, _, err := p.grabPos(nextTopPoss[i])
		if err != nil {
			return err
		}
		if nt == nil {
			return fmt.Errorf("want top %d at %d but nil", i, nextTopPoss[i])
		}
		if ntsib == nil {
			// when turning a node into a top, it's "nieces" are really children,
			// so should become it's sibling's nieces.
			nt.chop()
		} else {
			nt.niece = ntsib.niece
		}
		nexTops[i] = *nt
	}
	p.numLeaves = nextNumLeaves
	reversePolNodeSlice(nexTops)
	p.tops = nexTops
	return nil
}

func (p *Pollard) hnFromPos(pos uint64) (*hashableNode, error) {
	if !inForest(pos, p.numLeaves, p.height()) {
		// fmt.Printf("HnFromPos %d out of forest\n", pos)
		return nil, nil
	}
	_, _, hn, err := p.grabPos(pos)
	if err != nil {
		return nil, err
	}
	hn.position = up1(pos, p.height())
	return hn, nil
}

// swapNodes swaps the nodes at positions a and b.
// returns a hashable node with b, bsib, and bpar
func (p *Pollard) swapNodes(s arrow, height uint8) (*hashableNode, error) {
	if !inForest(s.from, p.numLeaves, p.height()) ||
		!inForest(s.to, p.numLeaves, p.height()) {
		return nil, fmt.Errorf("swapNodes %d %d out of bounds", s.from, s.to)
	}

	if p.positionMap != nil {
		a := childMany(s.from, height, p.height())
		b := childMany(s.to, height, p.height())
		run := uint64(1 << height)
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

	// fmt.Printf("swapNodes swapping a %d %x with b %d %x\n",
	// r.from, a.data[:4], r.to, b.data[:4])
	bhn.position = up1(s.to, p.height())
	// do the actual swap here
	err = polSwap(a, asib, b, bsib)
	if err != nil {
		return nil, err
	}
	if bhn.sib.niece[0].data == empty || bhn.sib.niece[1].data == empty {
		bhn = nil // we can't perform this hash as we don't know the children
	}
	return bhn, nil
}

// grabPos is like descendToPos but simpler.  Returns the thing you asked for,
// as well as its sibling.  And a hashable node for the position ABOVE pos.
// And an error if it can't get it.
// NOTE errors are not exhaustive; could return garbage without an error
func (p *Pollard) grabPos(
	pos uint64) (n, nsib *polNode, hn *hashableNode, err error) {
	tree, branchLen, bits := detectOffset(pos, p.numLeaves)
	// fmt.Printf("grab %d, tree %d, bl %d bits %x\n", pos, tree, branchLen, bits)
	if tree >= uint8(len(p.tops)) {
		err = fmt.Errorf("want tree %d but only %d trees", tree, len(p.tops))
		return
	}
	n, nsib = &p.tops[tree], &p.tops[tree]
	hn = &hashableNode{dest: n, sib: nsib}
	for h := branchLen - 1; h != 255; h-- { // go through branch
		lr := uint8(bits>>h) & 1
		if h == 0 { // if at bottom, done
			hn.dest = nsib // this is kind of confusing eh?
			hn.sib = n     // but yeah, switch siblingness
			n, nsib = n.niece[lr^1], n.niece[lr]
			if nsib == nil || n == nil {
				// fmt.Printf("gave up ")
				return // give up and don't make hashable node
			}
			// fmt.Printf("h%d n %x nsib %x\n", h, n.data[:4], nsib.data[:4])
			return
		}
		// if a sib doesn't exist, need to create it and hook it in
		if n.niece[lr^1] == nil {
			n.niece[lr^1] = new(polNode)
		}
		n, nsib = n.niece[lr], n.niece[lr^1]
		// fmt.Printf("h%d n %x nsib %x npar %x\n",
		// 	h, n.data[:4], nsib.data[:4], npar.data[:4])
		if n == nil {
			// if a node doesn't exist, crash
			err = fmt.Errorf("grab %d nil neice at height %d", pos, h)
			return
		}
	}
	return // only happens when returning a top
}

// DescendToPos returns the path to the target node, as well as the sibling
// path.  Retruns paths in bottom-to-top order (backwards)
// sibs[0] is the node you actually asked for
func (p *Pollard) descendToPos(pos uint64) ([]*polNode, []*polNode, error) {
	// interate to descend.  It's like the leafnum, xored with ...1111110
	// so flip every bit except the last one.
	// example: I want leaf 12.  That's 1100.  xor to get 0010.
	// descent 0, 0, 1, 0 (left, left, right, left) to get to 12 from 30.
	// need to figure out offsets for smaller trees.

	if !inForest(pos, p.numLeaves, p.height()) {
		//	if pos >= (p.numLeaves*2)-1 {
		return nil, nil,
			fmt.Errorf("OOB: descend to %d but only %d leaves", pos, p.numLeaves)
	}

	// first find which tree we're in
	tNum, branchLen, bits := detectOffset(pos, p.numLeaves)
	//	fmt.Printf("DO pos %d top %d bits %d branlen %d\n", pos, tNum, bits, branchLen)
	n := &p.tops[tNum]
	if branchLen > 64 {
		return nil, nil, fmt.Errorf("dtp top %d is nil", tNum)
	}

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

// toFull takes a pollard and converts to a forest.
// For debugging and seeing what pollard is doing since there's already
// a good toString method for  forest.
//func (p *Pollard) toFull() (*Forest, error) {
func (p *Pollard) toFull() (*Forest, error) {

	ff := NewForest(nil)
	ff.height = p.height()
	ff.numLeaves = p.numLeaves
	ff.data = new(ramForestData)
	ff.data.resize(2 << ff.height)
	if p.numLeaves == 0 {
		return ff, nil
	}

	//	for topIdx, top := range p.tops {
	//	}
	for i := uint64(0); i < (2<<ff.height)-1; i++ {
		_, sib, err := p.descendToPos(i)
		if err != nil {
			//	fmt.Printf("can't get pos %d: %s\n", i, err.Error())
			continue
			//			return nil, err
		}
		if sib[0] != nil {
			ff.data.write(i, sib[0].data)
			//	fmt.Printf("wrote leaf pos %d %04x\n", i, sib[0].data[:4])
		}

	}

	return ff, nil
}

//func (p *Pollard) ToString() string {
func (p *Pollard) ToString() string {
	f, err := p.toFull()
	if err != nil {
		return err.Error()
	}
	return f.ToString()
}

// equalToForest checks if the pollard has the same leaves as the forest.
// doesn't check tops and stuff
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
