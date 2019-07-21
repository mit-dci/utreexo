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

	currentUndo []undo

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

func (f *Forest) Remove(dels []uint64) ([]undo, error) {

	undos, err := f.removev2(dels)
	if err != nil {
		return nil, err
	}

	err = f.reHash()
	if err != nil {
		return nil, err
	}

	return undos, nil
}

// rewriting the remove func here then can delete the old one.
// the old one is just too messy.
func (f *Forest) removev2(dels []uint64) ([]undo, error) {

	if uint64(len(dels)) > f.numLeaves {
		return nil, fmt.Errorf("%d deletions but forest has %d leaves",
			len(dels), f.numLeaves)
	}

	// check that all dels are there & mark for deletion
	for _, dpos := range dels {
		if dpos > f.numLeaves {
			return nil, fmt.Errorf(
				"Trying to delete leaf at %d, beyond max %d", dpos, f.numLeaves)
		}
		// clear all entries from positionMap as they won't be needed any more
		fmt.Printf(" deleted %d %x from positionMap\n", dpos, f.forest[dpos][:4])
		delete(f.positionMap, f.forest[dpos].Mini())
	}

	f.currentUndo = []undo{}

	var moveDirt []uint64
	var hashDirt []uint64

	stashes, moves := removeTransform(dels, f.numLeaves, f.height)

	var stashSlice []tStash

	for h := uint8(0); h < f.height; h++ {
		// go through moves for this height
		for len(moves) > 0 && detectHeight(moves[0].to, f.height) == h {
			fmt.Printf("mv %d -> %d\n", moves[0].from, moves[0].to)
			err := f.moveSubtree(moves[0])
			if err != nil {
				return nil, err
			}

			dirt := up1(moves[0].to, f.height)
			lmvd := len(moveDirt)
			// the dirt returned by moveNode is always a parent so can never be 0
			if inForest(dirt, f.numLeaves) &&
				(lmvd == 0 || moveDirt[lmvd-1] != dirt) {
				fmt.Printf("h %d mv %d to moveDirt \n", h, dirt)
				moveDirt = append(moveDirt, dirt)
			}
			moves = moves[1:]
		}

		// then the stash on this height.  (There can be only 1)
		for len(stashes) > 0 && detectHeight(stashes[0].to, f.height) == h {
			fmt.Printf("stash %d -> %d\n", stashes[0].from, stashes[0].to)
			stash, err := f.getSubTree(stashes[0].from, true)
			if err != nil {
				return nil, fmt.Errorf("stash h %d %s", h, err)
			}
			stashSlice = append(stashSlice, stash.toTStash(stashes[0].to))
			stashes = stashes[1:]
		}

		// hash dirt for this height
		curDirt := mergeSortedSlices(moveDirt, hashDirt)
		moveDirt, hashDirt = []uint64{}, []uint64{}
		// if we're not at the top, and there's curDirty left, hash
		if h < f.height-1 {
			fmt.Printf("h %d curDirt %v\n", h, curDirt)
			for _, pos := range curDirt {

				lpos := child(pos, f.height)
				f.forest[pos] = Parent(f.forest[lpos], f.forest[lpos^1])
				f.HistoricHashes++

				parPos := up1(pos, f.height)
				lhd := len(hashDirt)
				// add parent to end of dirty slice if it's not already there
				if inForest(parPos, f.numLeaves) &&
					(lhd == 0 || hashDirt[lhd-1] != parPos) {
					fmt.Printf("h %d hash %d to hashDirt \n", h, parPos)
					hashDirt = append(hashDirt, parPos)
				}
			}
		}
	}

	// move subtrees from the stash to where they should go
	for _, tstash := range stashSlice {
		err := f.writeSubtree(rootStash{vals: tstash.vals}, tstash.dest)
		if err != nil {
			return nil, err
		}
	}
	f.numLeaves -= uint64(len(dels))
	return nil, nil
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
func (f *Forest) moveSubtree(m move) error {
	starttime := time.Now()
	fromHeight := detectHeight(m.from, f.height)
	toHeight := detectHeight(m.to, f.height)
	if fromHeight != toHeight {
		return fmt.Errorf("moveSubtree: mismatch heights from %d to %d",
			fromHeight, toHeight)
	}

	ms := subTreePositions(m.from, m.to, f.height)
	for j, mv := range ms {

		if f.forest[mv.from] == empty {
			return fmt.Errorf("move from %d but empty", mv.from)
		}

		if j < (1 << toHeight) { // we're on the bottom row
			f.positionMap[f.forest[mv.from].Mini()] = m.to
			//			fmt.Printf("wrote posmap pos %d\n", to)
			// store undo data if this is a leaf
			var u undo
			u.Hash = f.forest[mv.to]
			u.move = move{from: mv.to, to: mv.from}
			f.currentUndo = append(f.currentUndo, u)
		}

		// do the actual move
		fmt.Printf("mvsubtree write %x pos %d\n", f.forest[mv.from][:4], mv.to)
		f.forest[mv.to] = f.forest[mv.from]

		// clear out the place it moved from
		f.forest[mv.from] = empty

		if f.dirtyMap[mv.from] {
			f.dirtyMap[mv.to] = true
			if bridgeVerbose {
				fmt.Printf("movesubtree set pos %d to dirty, unset %d\n", m.to, m.from)
			}
			delete(f.dirtyMap, mv.from)
		}

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
		// node that the dirty positions are appended IN ORDER.
		// we can use that and don't have to sort through when we're
		// re-writing the dirtiness back
		if f.dirtyMap[m.from] {
			stash.dirts = append(stash.dirts, i)
			if del {
				delete(f.dirtyMap, m.from)
				if bridgeVerbose {
					fmt.Printf("getSubTree unset %d dirty\n", m.from)
				}
			}
		}

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

		if i < (1 << subheight) { // we're on the bottom row
			f.positionMap[stash.vals[i].Mini()] = m.to
			//			f.positionMap[stash.vals[i].Mini()] = to
			//			fmt.Printf("wrote posmap pos %d\n", to)
		}
		if len(stash.dirts) > 0 && stash.dirts[0] == i {
			f.dirtyMap[m.to] = true
			stash.dirts = stash.dirts[1:]
			if bridgeVerbose {
				fmt.Printf("writesubtree set pos %d to dirty\n", m.to)
			}
		}

	}
	return nil
}

// Add adds leaves to the forest.  This is the easy part.
func (f *Forest) Add(adds []LeafTXO) error {
	err := f.addInternal(adds)
	if err != nil {
		return err
	}
	return f.reHash()
}

// Add adds leaves to the forest.  This is the easy part.
func (f *Forest) addInternal(adds []LeafTXO) error {

	// add adds on the bottom right
	for _, add := range adds {
		fmt.Printf("forest add %x %d\t", add.Hash[:4], f.numLeaves)
		// watch out for off-by-1 here
		f.forest[f.numLeaves] = add.Hash
		f.positionMap[add.Mini()] = f.numLeaves
		f.dirtyMap[f.numLeaves] = true
		if bridgeVerbose {
			fmt.Printf("add set %d to dirty\n", f.numLeaves)
		}
		f.numLeaves++
	}
	//	fmt.Printf("forest now has %d leaves, %d height\n", f.numLeaves, f.height)
	return nil
	//	return f.reHash()
}

// Modify changes the forest, adding and deleting leaves and updating internal nodes.
// Note that this does not modify in place!  All deletes occur simultaneous with
// adds, which show up on the right.
// Also, the deletes need there to be correct proof data, so you should first call Verify().
func (f *Forest) Modify(adds []LeafTXO, dels []uint64) (*blockUndo, error) {

	bu := new(blockUndo)
	bu.adds = uint32(len(adds))

	delta := uint64(len(adds) - len(dels)) // watch 32/64 bit
	// remap to expand the forest if needed
	for f.numLeaves+delta > 1<<f.height {
		fmt.Printf("current cap %d need %d\n",
			1<<f.height, f.numLeaves+delta)
		err := f.reMap(f.height + 1)
		if err != nil {
			return nil, err
		}
	}

	fmt.Printf("pre remove %s\n", f.ToString())
	undos, err := f.removev2(dels)
	if err != nil {
		return nil, err
	}

	err = f.addInternal(adds)
	if err != nil {
		return nil, err
	}

	err = f.reHash()
	if err != nil {
		return nil, err
	}

	bu.undos = undos

	return bu, nil
}

// reHash recomputes all hashes above the first floor.
// now mapless (other than dirtymap) and looks parallelizable
func (f *Forest) reHash() error {
	// a little ugly but nothing to do in this case
	if f.height == 0 {
		return nil
	}
	if bridgeVerbose {
		s := f.ToString()
		fmt.Printf("\t\t\t====== tree rehash complete:\n")
		fmt.Printf(s)
	}
	starttime := time.Now()
	// check for tree tops
	tops, topheights := getTopsReverse(f.numLeaves, f.height)

	if bridgeVerbose {
		fmt.Printf("tops: %v\n", tops)
		fmt.Printf("rehash %d dirty: ", len(f.dirtyMap))
		for p, _ := range f.dirtyMap {
			fmt.Printf(" %d", p)
		}
		fmt.Printf("\n")
	}

	i := 0
	// less ugly!  turn the dirty map into a slice, sort it, and then
	// chop it up into rows
	// For more fancyness, use multiple goroutines for the hash part
	dirtySlice := make([]uint64, len(f.dirtyMap))
	for pos, _ := range f.dirtyMap {
		dirtySlice[i] = pos
		i++
	}
	sortUint64s(dirtySlice)

	dirty2d := make([][]uint64, f.height)
	h := uint8(0)
	dirtyRemaining := 0
	for _, pos := range dirtySlice {
		dHeight := detectHeight(pos, f.height)
		// increase height if needed
		for h < dHeight {
			h++
		}
		if bridgeVerbose {
			fmt.Printf("h %d\n", h)
		}
		dirty2d[h] = append(dirty2d[h], pos)
		dirtyRemaining++
	}

	// this is basically the same as VerifyBlockProof.  Could maybe split
	// it to a separate function to reduce redundant code..?
	// nah but pretty different beacuse the dirtyMap has stuff that appears
	// halfway up...

	var currentRow, nextRow []uint64

	// floor by floor
	for h = uint8(0); h < f.height; h++ {
		if bridgeVerbose {
			fmt.Printf("dirty %v\ncurrentRow %v\n", dirty2d[h], currentRow)
		}
		// merge nextRow and the dirtySlice.  They're both sorted so this
		// should be quick.  Seems like a CS class kindof algo but who knows.
		// Should be O(n) anyway.

		currentRow = mergeSortedSlices(currentRow, dirty2d[h])
		dirtyRemaining -= len(dirty2d[h])
		if dirtyRemaining == 0 && len(currentRow) == 0 {
			// done hashing early
			break
		}

		for i, pos := range currentRow {
			// skip if next is sibling
			if i+1 < len(currentRow) && currentRow[i]|1 == currentRow[i+1] {
				continue
			}
			// also skip if this is a top
			if pos == tops[0] {
				continue
			}

			right := pos | 1
			left := right ^ 1
			parpos := up1(left, f.height)
			leftHash := f.forest[left]
			rightHash := f.forest[right]
			if bridgeVerbose {
				fmt.Printf("bridge hash %d %04x, %d %04x -> %d\n",
					left, leftHash[:4], right, rightHash[:4], parpos)
			}
			par := Parent(leftHash, rightHash)
			f.HistoricHashes++

			f.forest[parpos] = par
			nextRow = append(nextRow, parpos)
		}
		if topheights[0] == h {
			tops = tops[1:]
			topheights = topheights[1:]
		}
		currentRow = nextRow
		nextRow = []uint64{}
	}

	// all clean
	f.dirtyMap = make(map[uint64]bool)

	donetime := time.Now()
	f.TimeInHash += donetime.Sub(starttime)
	if bridgeVerbose {
		s := f.ToString()
		fmt.Printf("\t\t\t====== tree rehash complete:\n")
		fmt.Printf(s)
	}
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
			// not 100% sure this is needed.  Maybe for dirty up higher
			if f.dirtyMap[(pos>>1)+x] {
				f.dirtyMap[pos+x] = true
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
		delete(f.dirtyMap, x)
	}

	f.height = destHeight

	//	fmt.Printf("forest slice len %d\n", len(f.forest))
	//	s := f.ToString()
	//	fmt.Printf(s)

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

	for i, _ := range tops {
		tops[i] = f.forest[topposs[i]]
	}

	return tops
}

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
	var empty Hash
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
