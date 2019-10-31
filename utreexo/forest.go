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

	err := f.removev3(dels)
	if err != nil {
		return err
	}

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
		// fmt.Printf(" deleted %d %x from positionMap\n", dpos, f.forest[dpos][:4])
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
			// from as well?
			dirt = append(dirt, s.to)
			if s.from < nextNumLeaves {
				dirt = append(dirt, s.from)
			}
		}
	}
	// go through dirt and update map
	for _, d := range dirt {
		// everything that moved needs to have its position updated in the map
		// TODO does it..?
		m := f.forest[d].Mini()
		oldpos := f.positionMap[m]
		if oldpos != d {
			fmt.Printf("update map %x %d to %d\n", m[:4], oldpos, d)
			delete(f.positionMap, m)
			f.positionMap[m] = d
		}
	}

	f.numLeaves = nextNumLeaves
	// f.cleanup()

	err := f.sanity()
	if err != nil {
		return err
	}
	return f.reHash(dirt)
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
func (f *Forest) Modify(adds []LeafTXO, dels []uint64) (*undoBlock, error) {
	numdels, numadds := uint64(len(dels)), uint64(len(adds))
	delta := numadds - numdels // watch 32/64 bit
	// remap to expand the forest if needed
	for f.numLeaves+delta > 1<<f.height {
		fmt.Printf("current cap %d need %d\n",
			1<<f.height, f.numLeaves+delta)
		err := f.reMap(f.height + 1)
		if err != nil {
			return nil, err
		}
	}

	// v3 should do the exact same thing as v2 now
	err := f.removev3(dels)
	if err != nil {
		return nil, err
	}
	// save the leaves past the edge for undo
	// dels hasn't been mangled by remove up above, right?
	// BuildUndoData takes all the stuff swapped to the right by removev3
	// and saves it in the order it's in, which should make it go back to
	// the right place when it's swapped in reverse
	ub := f.BuildUndoData(numadds, dels)

	f.addv2(adds)

	fmt.Printf("done modifying block, added %d\n", len(adds))
	// fmt.Printf("post add %s\n", f.ToString())
	// for m, p := range f.positionMap {
	// 	fmt.Printf("%x @%d\t", m[:4], p)
	// }
	// fmt.Printf("\n")
	err = f.sanity()
	return ub, err
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

// sanity checks forest sanity: does numleaves make sense, and are the tops
// populated?
func (f *Forest) sanity() error {

	if f.numLeaves > 1<<f.height {
		return fmt.Errorf("forest has %d leaves but insufficient height %d",
			f.numLeaves, f.height)
	}
	tops, _ := getTopsReverse(f.numLeaves, f.height)
	for _, t := range tops {
		if f.forest[t] == empty {
			return fmt.Errorf("Forest has %d leaves %d tops, but top @%d is empty",
				f.numLeaves, len(tops), t)
		}
	}
	if uint64(len(f.positionMap)) > f.numLeaves {
		return fmt.Errorf("sanity: positionMap %d leaves but forest %d leaves",
			len(f.positionMap), f.numLeaves)
	}

	return nil
}

// PosMapSanity is costly / slow: check that everything in posMap is correct
func (f *Forest) PosMapSanity() error {
	for i := uint64(0); i < f.numLeaves; i++ {
		if f.positionMap[f.forest[i].Mini()] != i {
			return fmt.Errorf("positionMap error: map says %x @%d but @%d",
				f.forest[i][:4], f.positionMap[f.forest[i].Mini()], i)
		}
	}
	return nil
}

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
