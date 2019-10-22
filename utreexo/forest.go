package utreexo

import (
	"fmt"
	"time"
)

// A FullForest is the entire accumulator of the UTXO set. This is
// what the bridge node stores.  Everything is always full.

/*
The forest is structured in the space of a tree numbered from the bottom left,
taking up the space of a perfect tree that can contain the whole forest.
This means that in most cases there will be null nodes in the tree.
That's OK; it helps reduce renumbering nodes and makes it easier to think about
addressing.  It also might work well for on-disk serialization.
There might be a better / optimal way to do this but it seems OK for now.
*/

// Forest :
type Forest struct {
	numLeaves uint64 // number of leaves in the forest (bottom row)
	// the full tree doesn't store the top roots as it has everything.  Can be
	// calculated from the forestMap.

	// height of the forest.  NON INTUITIVE!
	// When there is only 1 tree in the forest, it is equal to the height of
	// that tree (2**n nodes).  If there are multiple trees, fullHeight will
	// be 1 higher than the highest tree in the forest.
	// also, note that this is always round-up 2**numleaves.... so we don't
	// really need it.
	height uint8

	// moving to slice based forest.  more efficient, can be moved to
	// an on-disk file more easily (the subtree stuff should be changed
	// at that point to do runs of i/o).  Not sure about "deleting" as it
	// might not be needed at all with a slice.
	forest []Hash

	positionMap map[MiniHash]uint64 // map from hashes to positions.
	// Inverse of forestMap for leaves.

	// ----- it's kind of ugly to put this here but a bunch of places need it;
	// basically modify and all sub-functions.  Not for prove/verify
	// though.  But those are super simple.

	// TODO can probably get rid of a map here and make it a slice
	// only odd nodes should ever get dirty
	dirtyMap map[uint64]bool

	// -------------------- following are just for testing / benchmarking
	// how many hashes this forest has computed
	HistoricHashes uint64

	// time taken in Remove() function
	TimeRem time.Duration
	// of which time in the moveSubTree() function
	TimeMST time.Duration

	// time taken in hash operations (reHash)
	TimeInHash time.Duration

	// time taken in Prove operations
	TimeInProve time.Duration

	// the time taken in verify operations
	TimeInVerify time.Duration
}

// NewForest :
func NewForest() *Forest {
	f := new(Forest)
	f.numLeaves = 0
	f.height = 0
	f.forest = make([]Hash, 1) // height 0 forest has 1 node
	//	f.forestMapx = make(map[uint64]Hash)
	f.positionMap = make(map[MiniHash]uint64)
	f.dirtyMap = make(map[uint64]bool)

	return f
}

const sibSwap = false
const bridgeVerbose = true

// empty is needed for detection (to find errors) but I'm not sure it's needed
// for deletion.  I think you can just leave garbage around, as it'll either
// get immediately overwritten, or it'll be out to the right, beyond the edge
// of the forest
var empty [32]byte

// Remove :
func (f *Forest) Remove(dels []uint64) error {

	err := f.removev2(dels)
	if err != nil {
		return err
	}

	return nil
}

// rewriting the remove func here then can delete the old one.
// the old one is just too messy.
func (f *Forest) removev2(dels []uint64) error {

	if uint64(len(dels)) > f.numLeaves {
		return fmt.Errorf("%d deletions but forest has %d leaves",
			len(dels), f.numLeaves)
	}

	// check that all dels are there & mark for deletion
	for _, dpos := range dels {
		if dpos > f.numLeaves {
			return fmt.Errorf(
				"Trying to delete leaf at %d, beyond max %d", dpos, f.numLeaves)
		}
		// clear all entries from positionMap as they won't be needed any more
		fmt.Printf(" deleted %d %x from positionMap\n", dpos, f.forest[dpos][:4])
		delete(f.positionMap, f.forest[dpos].Mini())
	}

	var moveDirt, hashDirt []uint64

	fmt.Printf("frst call remTr %d %d %d\n", dels, f.numLeaves, f.height)
	stashes, moves := removeTransform(dels, f.numLeaves, f.height)

	var stashSlice []tStash

	for h := uint8(0); h < f.height; h++ {
		// hash first for this height
		curDirt := mergeSortedSlices(moveDirt, hashDirt)
		moveDirt, hashDirt = []uint64{}, []uint64{}
		// if we're not at the top, and there's curDirt remaining, hash
		fmt.Printf("h %d curDirt %v\n", h, curDirt)
		for _, pos := range curDirt {
			lpos := child(pos, f.height)
			// fmt.Printf("%d %x, %d %x\n",
			// lpos, f.forest[lpos][:4], lpos^1, f.forest[lpos^1][:4])
			if f.forest[lpos] == empty || f.forest[lpos^1] == empty {
				f.forest[pos] = empty
				fmt.Printf("clear pos %d due to empty child\n", pos)
			} else {
				f.forest[pos] = Parent(f.forest[lpos], f.forest[lpos^1])
				f.HistoricHashes++
				parPos := up1(pos, f.height)
				lhd := len(hashDirt)
				// add parent to end of dirty slice if it's not already there
				if lhd == 0 || hashDirt[lhd-1] != parPos {
					fmt.Printf("for h %d pos %d hash %d to hashDirt \n",
						h, pos, parPos)
					hashDirt = append(hashDirt, parPos)
				}
			}
		}

		// go through moves for this height
		for len(moves) > 0 && detectHeight(moves[0].to, f.height) == h {
			cmove := moves[0]
			if h == 0 {
				// add to undo list
				// umove := move{from: cmove.to, to: cmove.from}
				// f.currentUndo = append(f.currentUndo,
				// undo{Hash: f.forest[umove.to], move: umove})
			}

			// fmt.Printf("mv %d -> %d\n", moves[0].from, moves[0].to)
			err := f.moveSubtree(cmove)
			if err != nil {
				return err
			}
			dirt := up1(cmove.to, f.height)
			lmvd := len(moveDirt)
			// the dirt returned by moveSubtree is always a parent so can never be 0
			if lmvd == 0 || moveDirt[lmvd-1] != dirt {
				// fmt.Printf("h %d mv %d to moveDirt \n", h, dirt)
				moveDirt = append(moveDirt, dirt)
			}
			moves = moves[1:]
		}

		// then the stash on this height.  (There can be only 1)
		if len(stashes) > 0 && detectHeight(stashes[0].to, f.height) == h {
			if stashes[0].from != stashes[0].to {
				fmt.Printf("stash %d -> %d\n", stashes[0].from, stashes[0].to)
				stash, err := f.getSubTree(stashes[0].from, true)
				if err != nil {
					return fmt.Errorf("stash h %d %s", h, err)
				}
				stashSlice = append(stashSlice, stash.toTStash(stashes[0].to))
			}
			stashes = stashes[1:]
		}
	}

	// move subtrees from the stash to where they should go
	for _, tstash := range stashSlice {
		err := f.writeSubtree(rootStash{vals: tstash.vals}, tstash.dest)
		if err != nil {
			return err
		}
	}
	f.numLeaves -= uint64(len(dels))
	return nil
}

// removev3 uses top down swaps and hopefully works the exact same as before
// top down swaps are better suited to undoing deletions
func (f *Forest) removev3(dels []uint64) error {

	if uint64(len(dels)) > f.numLeaves {
		return fmt.Errorf("%d deletions but forest has %d leaves",
			len(dels), f.numLeaves)
	}
	nextNumLeaves := f.numLeaves - uint64(len(dels))

	// check that all dels are there & mark for deletion
	for _, dpos := range dels {
		if dpos > f.numLeaves {
			return fmt.Errorf(
				"Trying to delete leaf at %d, beyond max %d", dpos, f.numLeaves)
		}
		// clear all entries from positionMap as they won't be needed any more
		fmt.Printf(" deleted %d %x from positionMap\n", dpos, f.forest[dpos][:4])
		delete(f.positionMap, f.forest[dpos].Mini())
	}

	var dirt []uint64

	fmt.Printf("v3 topDownTransform %d %d %d\n", dels, f.numLeaves, f.height)
	swaps := floorTransform(dels, f.numLeaves, f.height)
	fmt.Printf("v3 got swaps: %v\n", swaps)

	// TODO definitely not how to do this, way inefficient
	// dirt should be on the top, this is redundant
	for _, s := range swaps {
		f.forest[s.from], f.forest[s.to] = f.forest[s.to], f.forest[s.from]
		if s.to < nextNumLeaves {
			dirt = append(dirt, s.to)
			if s.from < nextNumLeaves {
				dirt = append(dirt, s.from)
			}
		}
	}

	f.numLeaves = nextNumLeaves
	f.cleanup()
	return f.reHash(dirt)
	// if h == 0 {
	// add to undo list
	// umove := move{from: cmove.to, to: cmove.from}
	// f.currentUndo = append(f.currentUndo,
	// undo{Hash: f.forest[umove.to], move: umove})
	// }

	// for _, pos := range dirt {
	// 	lpos := child(pos, f.height)
	// 	// fmt.Printf("%d %x, %d %x\n",
	// 	// lpos, f.forest[lpos][:4], lpos^1, f.forest[lpos^1][:4])
	// 	if f.forest[lpos] == empty || f.forest[lpos^1] == empty {
	// 		f.forest[pos] = empty
	// 		fmt.Printf("clear pos %d due to empty child\n", pos)
	// 	} else {
	// 		f.forest[pos] = Parent(f.forest[lpos], f.forest[lpos^1])
	// 		f.HistoricHashes++
	// 		parPos := up1(pos, f.height)
	// 		lhd := len(dirt)
	// 		// add parent to end of dirty slice if it's not already there
	// 		if lhd == 0 || dirt[lhd-1] != parPos {
	// 			fmt.Printf("pos %d hash %d to hashDirt \n", pos, parPos)
	// 			dirt = append(dirt, parPos)
	// 		}
	// 	}
	// }
}

// cleanup removes extraneous hashes from the forest.  Currently only the bottom
func (f *Forest) cleanup() {
	for p := f.numLeaves; p < 1<<f.height; p++ {
		f.forest[p] = empty
	}
}

type rootStash struct {
	vals  []Hash
	dirts []int // I know, I know, it's ugly but it's for slice indexes...
}

func (r *rootStash) toTStash(to uint64) tStash {
	return tStash{vals: r.vals, dest: to}
}

// tStash is a stash used with removeTransform
type tStash struct {
	vals []Hash
	dest uint64
}

// TODO
/*  ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
root in place can break when something above you moves in, and then
that tree moves again another row up, exposing the root.  This
requires at least 3 populated rows above you - one to clobber you,
one to move back out of the way, and another on top of that --
(nobody moves unless there was something above them)
There may be more restrictions - it seems like a hard edge case
to trigger, and it's potentially costly to stash something that
doesn't need to be stashed.  For now stash everything, even if it roots in place.
Definitely can be improved / optimized.
*/

// moveSubtree moves a node, and all its children, from one place to another,
// and deletes everything at the prior location
// This is like get and write subtree but moving directly instead of stashing
func (f *Forest) moveSubtree(a arrow) error {
	fmt.Printf("movesubtree %d -> %d\n", a.from, a.to)
	starttime := time.Now()
	fromHeight := detectHeight(a.from, f.height)
	toHeight := detectHeight(a.to, f.height)
	if fromHeight != toHeight {
		return fmt.Errorf("moveSubtree: mismatch heights from %d to %d",
			fromHeight, toHeight)
	}

	ms := subTreePositions(a.from, a.to, f.height)
	for _, submove := range ms {

		if f.forest[submove.from] == empty {
			return fmt.Errorf("move from %d but empty", submove.from)
		}

		if submove.from < f.numLeaves { // we're on the bottom row
			f.positionMap[f.forest[submove.from].Mini()] = submove.to
			fmt.Printf("map %x @ %d\n", f.forest[submove.from][:4], submove.to)
			// store undo data if this is a leaf
			// var u undo
			// u.Hash = f.forest[submove.to]
			// u.move = move{from: submove.to, to: submove.from}
			// f.currentUndo = append(f.currentUndo, u)
		}

		// do the actual move
		fmt.Printf("mvsubtree write %x pos %d\n",
			f.forest[submove.from][:4], submove.to)
		f.forest[submove.to] = f.forest[submove.from]

		// clear out the place it moved from
		f.forest[submove.from] = empty

	}

	donetime := time.Now()
	f.TimeMST += donetime.Sub(starttime)

	return nil
}

// getSubTree returns a subtree in []node format given a position in the forest
// deletes the subtree after reading it if del is true
func (f *Forest) getSubTree(src uint64, del bool) (rootStash, error) {
	var stash rootStash

	// get position listing of all nodes in subtree
	ms := subTreePositions(src, src, f.height)

	if src >= uint64(len(f.forest)) {
		return stash, fmt.Errorf("getSubTree: subtree %d not in len %d forest", src, len(f.forest))
	}
	if f.forest[src] == empty {
		return stash, fmt.Errorf("getSubTree: subtree %d empty", src)
	}

	stash.vals = make([]Hash, len(ms))

	// read from map and build slice in down to up order
	for i, m := range ms {
		stash.vals[i] = f.forest[m.from]

		if del {
			f.forest[m.from] = empty
			//			delete(f.forestMap, from)
		}
	}

	return stash, nil
}

// writeSubtree is like moveSubtree but writes it in from an argument
func (f *Forest) writeSubtree(stash rootStash, dest uint64) error {
	subheight := detectHeight(dest, f.height)
	if len(stash.vals) != (2<<subheight)-1 {
		return fmt.Errorf(
			"writeSubtree height %d but %d nodes in arg subtree (need %d)",
			subheight, len(stash.vals), (2<<subheight)-1)
	}

	ms := subTreePositions(dest, dest, f.height)
	// tos start at the bottom and move up, standard
	//	fmt.Printf("subtree size %d\n", len(tos))

	for i, m := range ms {
		f.forest[m.to] = stash.vals[i]

		if m.from < f.numLeaves { // we're on the bottom row
			f.positionMap[stash.vals[i].Mini()] = m.to
			//			f.positionMap[stash.vals[i].Mini()] = to
			fmt.Printf("wrote posmap pos %d\n", m.to)
		}
	}
	return nil
}

// Add adds leaves to the forest.  This is the easy part.
func (f *Forest) Add(adds []LeafTXO) {
	f.addv2(adds)
}

// Add adds leaves to the forest.  This is the easy part.
func (f *Forest) addv2(adds []LeafTXO) {

	for _, add := range adds {
		// fmt.Printf("adding %x pos %d\n", add.Hash[:4], f.numLeaves)
		f.positionMap[add.Mini()] = f.numLeaves

		tops, _ := getTopsReverse(f.numLeaves, f.height)
		pos := f.numLeaves
		n := add.Hash
		f.forest[pos] = n // write leaf
		for h := uint8(0); (f.numLeaves>>h)&1 == 1; h++ {
			// grab, pop, swap, hash, new
			top := f.forest[tops[h]] // grab
			//			fmt.Printf("grabbed %x from %d\n", top[:12], tops[h])
			n = Parent(top, n)       // hash
			pos = up1(pos, f.height) // rise
			f.forest[pos] = n        // write
			//			fmt.Printf("wrote %x to %d\n", n[:4], pos)
		}
		f.numLeaves++
	}
	return
}

// Modify changes the forest, adding and deleting leaves and updating internal nodes.
// Note that this does not modify in place!  All deletes occur simultaneous with
// adds, which show up on the right.
// Also, the deletes need there to be correct proof data, so you should first call Verify().
func (f *Forest) Modify(adds []LeafTXO, dels []uint64) error {

	delta := uint64(len(adds) - len(dels)) // watch 32/64 bit
	// remap to expand the forest if needed
	for f.numLeaves+delta > 1<<f.height {
		fmt.Printf("current cap %d need %d\n",
			1<<f.height, f.numLeaves+delta)
		err := f.reMap(f.height + 1)
		if err != nil {
			return err
		}
	}

	err := f.removev2(dels)
	if err != nil {
		return err
	}

	f.addv2(adds)

	fmt.Printf("done modifying block, added %d\n", len(adds))
	fmt.Printf("post add %s\n", f.ToString())
	for m, p := range f.positionMap {
		fmt.Printf("%x @%d\t", m[:4], p)
	}
	fmt.Printf("\n")

	return nil
}

// reMap changes the height of the forest
func (f *Forest) reMap(destHeight uint8) error {

	if destHeight == f.height {
		return fmt.Errorf("can't remap %d to %d... it's the same",
			destHeight, destHeight)
	}

	if destHeight > f.height+1 || (f.height > 0 && destHeight < f.height-1) {
		return fmt.Errorf("changing by more than 1 height not programmed yet")
	}

	fmt.Printf("remap forest height %d -> %d\n", f.height, destHeight)

	// for height reduction
	if destHeight < f.height {
		return fmt.Errorf("height reduction not implemented")
	}
	// I don't think you ever need to remap down.  It really doesn't
	// matter.  Something to program someday if you feel like it for fun.

	// height increase

	f.forest = append(f.forest, make([]Hash, 1<<destHeight)...)

	pos := uint64(1 << destHeight) // leftmost position of row 1
	reach := pos >> 1              // how much to next row up
	// start on row 1, row 0 doesn't move
	for h := uint8(1); h < destHeight; h++ {
		runLength := reach >> 1
		for x := uint64(0); x < runLength; x++ {
			// ok if source position is non-empty
			ok := len(f.forest) > int((pos>>1)+x) &&
				f.forest[(pos>>1)+x] != empty
			src := f.forest[(pos>>1)+x]
			if ok {
				f.forest[pos+x] = src
			}
		}
		pos += reach
		reach >>= 1
	}

	// zero out (what is now the) right half of the bottom row
	//	copy(t.fs[1<<(t.height-1):1<<t.height], make([]Hash, 1<<(t.height-1)))
	for x := uint64(1 << f.height); x < 1<<destHeight; x++ {
		// here you may actually need / want to delete?  but numleaves
		// should still ensure that you're not reading over the edge...
		f.forest[x] = empty
	}

	f.height = destHeight
	return nil
}

/*

For re-arranging sub-trees in the full tree mode.
Usually you move things left, by subtracting from their position.
eg 27 moves to 24.  If that's at height 2, the nodes below at heights 1, 0 also
move.  The way they move: take the initial movement at the top (height 2),
in this case 2 (27-24 = 3)  At height 1, they will move -6 (-3 << 1) and at
height 0 they move -12 (-3 << 2).  Can't really do signed shifts but can subtract;
in the cases when subtrees move right, add instead.  Moving right is rare though.

*/

// GetTops returns all the tops of the trees
func (f *Forest) GetTops() []Hash {

	topposs, _ := getTopsReverse(f.numLeaves, f.height)
	tops := make([]Hash, len(topposs))

	for i := range tops {
		tops[i] = f.forest[topposs[i]]
	}

	return tops
}

// Stats :
func (f *Forest) Stats() string {

	s := fmt.Sprintf("numleaves: %d hashesever: %d posmap: %d forest: %d\n",
		f.numLeaves, f.HistoricHashes, len(f.positionMap), len(f.forest))

	s += fmt.Sprintf("\thashT: %.2f remT: %.2f (of which MST %.2f) proveT: %.2f",
		f.TimeInHash.Seconds(), f.TimeRem.Seconds(), f.TimeMST.Seconds(),
		f.TimeInProve.Seconds())
	return s
}

// ToString prints out the whole thing.  Only viable for small forests
func (f *Forest) ToString() string {

	fh := f.height
	// tree height should be 6 or less
	if fh > 6 {
		return "forest too big to print "
	}

	output := make([]string, (fh*2)+1)
	var pos uint8
	for h := uint8(0); h <= fh; h++ {
		rowlen := uint8(1 << (fh - h))

		for j := uint8(0); j < rowlen; j++ {
			var valstring string
			ok := len(f.forest) >= int(pos)
			if ok {
				val := f.forest[uint64(pos)]
				if val != empty {
					valstring = fmt.Sprintf("%x", val[:2])
				}
			}
			if valstring != "" {
				output[h*2] += fmt.Sprintf("%02d:%s ", pos, valstring)
			} else {
				output[h*2] += fmt.Sprintf("        ")
			}
			if h > 0 {
				//				if x%2 == 0 {
				output[(h*2)-1] += "|-------"
				for q := uint8(0); q < ((1<<h)-1)/2; q++ {
					output[(h*2)-1] += "--------"
				}
				output[(h*2)-1] += "\\       "
				for q := uint8(0); q < ((1<<h)-1)/2; q++ {
					output[(h*2)-1] += "        "
				}

				//				}

				for q := uint8(0); q < (1<<h)-1; q++ {
					output[h*2] += "        "
				}

			}
			pos++
		}

	}
	var s string
	for z := len(output) - 1; z >= 0; z-- {
		s += output[z] + "\n"
	}
	return s
}
