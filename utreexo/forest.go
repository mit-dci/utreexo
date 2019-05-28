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
const bridgeVerbose = false

// empty is needed for detection (to find errors) but I'm not sure it's needed
// for deletion.  I think you can just leave garbage around, as it'll either
// get immediately overwritten, or it'll be out to the right, beyond the edge
// of the forest
var empty [32]byte

func (f *Forest) Remove(dels []uint64) error {

	err := f.removeInternal(dels)
	if err != nil {
		return err
	}
	return f.reHash()

}

// Deletion and addition are ~completely separate operations in the forest,
// so lets break em into different methods.

// TODO use removeTransform here so we know we're doing the same thing

// Remove deletes nodes from the forest.  This is the hard part :)
// note that in the Forest method, we don't actually use the proofs,
// just the position being deleted

func (f *Forest) removeInternal(dels []uint64) error {
	starttime := time.Now()
	if len(dels) == 0 {
		return nil
	}
	numDeletions := uint64(len(dels))
	if numDeletions > f.numLeaves {
		return fmt.Errorf(
			"%d deletions but forest has %d leaves",
			len(dels), f.numLeaves)
	}

	//	var ds []uint64          // slice of positions of nodes to be deleted
	var up1DelSlice []uint64 // the next ds, one row up (in construction)

	// list of nodes in the tree to compute new hashes of
	//	var reHash []uint64

	// pre-processing steps: sanity checks

	// check that all dels are there & mark for deletion
	for _, dpos := range dels {
		if dpos > f.numLeaves {
			return fmt.Errorf(
				"Trying to delete leaf at %d, beyond max %d", dpos, f.numLeaves)
		}
		//		ds = append(ds, dpos)
		// have the forest so no need to populate with d.siblings

		// clear all entries from positionMap as they won't be needed any more
		// quick & easy; doesn't affcet the tree
		f.positionMap[f.forest[dpos].Mini()] = dpos
		//		f.delPosition()
	}

	// get tree tops.  these are dealt with differently...

	// TODO not sure this is needed... try to get rid of it
	rootPosMap := make(map[uint8]uint64)
	nextRootPosMap := make(map[uint8]uint64)
	// need a place to stash subtrees.  there's probably lots of ways to do a
	// better job of this.  pointers and stuff.  Have each height tree in a different
	// file or map, so that you don't have to move them twice.  Or keep subtrees
	// in serialized chunks.

	// the stash is a map of heights to stashes.  The stashes have slices
	// of hashes, from bottom left up to subroot.  Same ordering as the main forest
	// they also have dirty uint64s to indicate which hashes are dirty
	stashMap := make(map[uint8]rootStash)

	roots, rootHeights := getTopsReverse(f.numLeaves, f.height)
	nextRoots, nextHeights := getTopsReverse(f.numLeaves-uint64(len(dels)), f.height)
	// populate the map of root positions, and of root values
	// needed only to determine if you're deleting a root..?
	for i, r := range roots {
		rootPosMap[rootHeights[i]] = r
	}
	for i, r := range nextRoots {
		nextRootPosMap[nextHeights[i]] = r
	}
	//		fmt.Printf(" h:%d %d\t", detectHeight(t, f.height), t)
	// for which heights do we have a top

	// TODO
	// what happens if you delete everything? do we care?

	//	fmt.Printf("Have %d deletions:\n", len(ds))

	/* all these steps need to happen for every floor, starting at sorting, and
	including extracting siblings.

	Steps for each floor:
	Sort (maybe not needed on upper floors?) (but can't hurt)
	Extract twins (move twins up 1 & delete, leave non-twins)
	Swap / condense remaining, and move children. <- flag dirty here
	If there are an odd number remaining, move to / from right root

	The Extract Twins step maybe could be left till the end?
	There's 2 different ways to do this, and it looks like it changes the
	order, so maybe try both ways and see which works best...
	A: first remove / promote twins, then swap OCs to compact.
	B: swap orphans into empty twins.  Doable!  May lead to empty twins
	more on the right side, which is .. good?  B seems "simpler" in a way
	in that you're not treating twins differently.

	Dirty bits for what to rehash are only set in the swap phase.  In extract, there's
	no need to hash anything as both siblings are gone.  In root phase, when something
	is derooted it's marked dirty, but not when something is rooted.
	It needs to be a dirty[map] because when you move subtrees, dirty positions also
	need to move.
	*/

	// the main floor loop.
	// per row:
	// sort / delete / extract / swap / root / promote
	for h := uint8(0); h <= f.height; h++ {
		//		fmt.Printf("deleting at height %d of %d, %d deletions:\n",
		//			h, f.height, len(ds))
		// *** skip.  if there are no deletions at this height, we're done
		if len(dels) == 0 {
			break
		}

		// *** sort.  Probably pointless on upper floors..?
		sortUint64s(dels)

		// *** delete
		// actually delete everything first (floor 0)
		// everywhere else you delete, there should probably be a ^1
		// except no, there's places where you move subtrees around & delete em
		for _, d := range dels {
			// TODO probably don't need to do this.  well not all the time.
			// but some of the time maybe do,
			f.forest[d] = empty
			//			delete(f.forestMap, d)
		}
		// check for root deletion (it can only be the last one)
		thisRoot, ok := rootPosMap[h]
		if ok && thisRoot == dels[len(dels)-1] {
			dels = dels[:len(dels)-1] // pop off
			delete(rootPosMap, h)
			//			delete(rootStash, h)
		}

		// *** extract / dedupe
		var twins []uint64
		twins, dels = ExtractTwins(dels)
		// run through the slice of deletions, and 'dedupe' by eliminating siblings
		// (if both siblings are gone, nothing needs to move)
		//		fmt.Printf("\nafter dedupe, %d deletions remain, %d twins\n",
		//			len(ds), len(twins))

		for _, sib := range twins {
			//			delete(f.forestMap, d)
			//			delete(f.forestMap, d|1)
			//			fmt.Printf("even twin %d\n", sib)
			up1DelSlice = append(up1DelSlice, up1(sib, f.height))
		}

		// *** swap
		for len(dels) > 1 {

			if sibSwap && dels[0]&1 == 0 { // if destination is even (left)
				err := f.moveSubtree(dels[0]^1, dels[0]) // swap siblings first
				// TODO do I need to pass dirtymap there? I think so, but not sure
				if err != nil {
					return err
				}
				// set destinationg to newly vacated right sibling
				dels[0] = dels[0] ^ 1
			}

			err := f.moveSubtree(dels[1]^1, dels[0])
			if err != nil {
				return err
			}
			// set dirty bit for destination
			f.dirtyMap[dels[0]] = true
			if bridgeVerbose {
				fmt.Printf("swap set pos %d to dirty\n", dels[0])
			}
			// deletion promotes to next row
			up1DelSlice = append(up1DelSlice, up1(dels[1], f.height))
			dels = dels[2:]
		}

		//		fmt.Printf("after swap, %d deletions remain\n", len(ds))

		// *** root
		// the rightmost element of this floor *is* a root.
		// If we're deleting it, delete it now; its presence is important for
		// subsequent swaps
		// scenarios: deletion is present / absent, and root is present / absent

		// deletion, root: deroot
		// deletion, no root: rootify (possibly in place)
		// no deletion, root: stash root (it *will* collapse left later)
		// no deletion, no root: nothing to do

		if len(dels) > 1 {
			return fmt.Errorf("%d deletions in root phase\n", len(dels))
		}

		// check if a root is present on this floor
		rootPos, rootPresent := rootPosMap[h]

		// the last remaining deletion (if exists) can swap with the root

		// weve already deleted roots either in the delete phase, so there can't
		// be a root here that we are deleting. (though maybe make sure?)
		// so the 2 possibilities are: 1) root exists and root subtree moves to
		// fill the last deletion (derooting), or 2) root does not exist and last
		// OC subtree moves to root position (rooting)
		var delPos uint64
		var haveDel bool
		if len(dels) == 1 {
			delPos = dels[0]
			haveDel = true
		}

		upDel, subStash, err := f.rootPhase(
			haveDel, rootPresent, delPos, rootPos, h)
		if err != nil {
			return err
		}

		if upDel != 0 {
			// if de-rooting, interpret "updel" as a dirty position
			if haveDel && rootPresent {
				f.dirtyMap[upDel] = true
				if bridgeVerbose {
					fmt.Printf("deroot set pos %d to dirty\n", upDel)
				}
			} else {
				// otherwise it's an upDel
				up1DelSlice = append(up1DelSlice, upDel)
			}
		}
		if len(subStash.vals) != 0 {
			stashMap[h] = subStash
		}

		// done with one row, set ds to the next slice
		dels = up1DelSlice
		up1DelSlice = []uint64{}
	}
	if len(dels) != 0 {
		return fmt.Errorf("finished deletion climb but %d deletion left", len(dels))
	}

	// move subtrees from the stash to where they should go
	for height, stash := range stashMap {
		destPos := nextRootPosMap[height]
		//		fmt.Printf("moving stashed subtree to h %d pos %d\n", height, destPos)

		err := f.writeSubtree(stash, destPos)
		if err != nil {
			return err
		}

		//		if height == 0 {
		// this shouldn't crash; subtree[0] should exist
		//			f.positionMap[subtree[0]] = destPos // == numleaves-1
		//		}
	}

	// deletes have been applied, reduce numLeaves
	f.numLeaves -= numDeletions

	donetime := time.Now()
	f.TimeRem += donetime.Sub(starttime)

	return nil
}

type rootStash struct {
	vals    []Hash
	dirts   []int // I know, I know, it's ugly but it's for slice indexes...
	forgets []int // yuck
}

// root phase is the most involved of the deletion phases.  broken out into its own
// method.  Returns a deletion and a stash.  If the deletion is 0 it's invalid
// as 0 can never be on a non-zero floor.
func (f *Forest) rootPhase(haveDel, haveRoot bool,
	delPos, rootPos uint64, h uint8) (uint64, rootStash, error) {

	var upDel uint64 // higher position to mark for deletion
	var stash rootStash
	// *** root
	// scenarios: deletion is present / absent, and root is present / absent

	// deletion, root: deroot, move to sibling
	// deletion, no root: rootify (possibly in place) & stash
	// no deletion, root: stash existing root (it *will* collapse left later)
	// no deletion, no root: nothing to do

	// weve already deleted roots either in the delete phase, so there can't
	// be a root here that we are deleting. (though maybe make sure?)

	if haveDel && haveRoot { // derooting.  simplest
		// root is present, move root to occupy the rightmost gap

		//		_, ok := f.forestMap[rootPos]
		if f.forest[rootPos] == empty {
			return 0, stash, fmt.Errorf("move from %d but empty", rootPos)
		}

		// move
		//		fmt.Printf("move root from %d to %d (derooting)\n", rootPos, delPos)

		if sibSwap && delPos&1 == 0 { // if destination is even (left)
			err := f.moveSubtree(delPos^1, delPos) // swap siblings first
			if err != nil {
				return 0, stash, err
			}
			// set destinationg to newly vacated right sibling
			delPos = delPos ^ 1 // |1 should also work
		}

		err := f.moveSubtree(rootPos, delPos)
		if err != nil {
			return 0, stash, err
		}
		// delPos | 1 is to ensure it's not 0; marking either sibling dirty works
		// which is maybe weird and confusing...
		return delPos | 1, stash, nil // done here? just return dirty position
	}

	if !haveDel && !haveRoot { // ok no that's even simpler
		return 0, stash, nil
	}

	var stashPos uint64

	// these are redundant, could just do if haveRoot / else here but helps to see
	// what's going on

	if !haveDel && haveRoot { // no deletion, root exists: stash it
		// if there are 0 deletions remaining we need to stash the
		// current root, because it will collapse leftward at the end as
		// deletions did occur on this floor.
		stashPos = rootPos
	}

	if haveDel && !haveRoot { // rooting
		// if there's a deletion, the thing to stash is its sibling
		stashPos = delPos ^ 1
		// mark parent for deletion. this happens even if the node
		// being promoted to root doesn't move
		upDel = up1(stashPos, f.height)
	}

	// .. even if the root is in the right place... it still needs to
	// be stashed.  Activity above it can overwrite it.

	// move either standing root or rightmost OC subtree to stash
	// but if this position is already in the right place, don't have to
	// stash it anywhere, so skip.  If skipped, a non-root node still
	// gets rooted, it just doesn't have to move.

	// read subtree.  In this case, also delete the subtree
	stash, err := f.getSubTree(stashPos, true)
	if err != nil {
		return 0, stash, err
	}

	// stash the hashes in down to up order, and also stash the dirty bits
	// in any order (slice of uint64s)

	//	fmt.Printf("moved position %d to h %d root stash\n", stashPos, h)
	return upDel, stash, nil
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
func (f *Forest) moveSubtree(from, to uint64) error {
	starttime := time.Now()
	fromHeight := detectHeight(from, f.height)
	toHeight := detectHeight(to, f.height)
	if fromHeight != toHeight {
		return fmt.Errorf("moveSubtree: mismatch heights from %d to %d",
			fromHeight, toHeight)
	}

	ms := subTreePositions(from, to, f.height)
	for j, m := range ms {

		if f.forest[m.from] == empty {
			return fmt.Errorf("move from %d but empty", from)
		}
		f.forest[m.to] = f.forest[m.from]

		if j < (1 << toHeight) { // we're on the bottom row
			f.positionMap[f.forest[m.to].Mini()] = m.to
			//			fmt.Printf("wrote posmap pos %d\n", to)
		}
		// also move position map for items on bottom row
		//		if detectHeight(from, f.height) == 0 {
		//			f.positionMap[f.forestMap[tos[j]]] = tos[j]
		//		}

		f.forest[m.from] = empty
		//		delete(f.forestMap, from)

		if f.dirtyMap[m.from] {
			f.dirtyMap[m.to] = true
			if bridgeVerbose {
				fmt.Printf("movesubtree set pos %d to dirty, unset %d\n", m.to, m.from)
			}
			delete(f.dirtyMap, m.from)
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

	if int(src) >= len(f.forest) || f.forest[src] == empty {
		return stash, fmt.Errorf("getSubTree: subtree %d not in forest", src)
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
func (f *Forest) Modify(adds []LeafTXO, dels []uint64) error {

	delta := len(adds) - len(dels) // watch 32/64 bit
	// remap to expand the forest if needed
	for int(f.numLeaves)+delta > 1<<f.height {
		//		fmt.Printf("current cap %d need %d\n",
		//			1<<f.height, (1<<f.height)+uint64(len(adds)))
		err := f.reMap(f.height + 1)
		if err != nil {
			return err
		}
	}

	// lookup where the deletions are
	/*
		delposs := make([]uint64, len(dels))
		for i, delhash := range dels {
			pos, err := f.getPosition(delhash.Mini())
			if err != nil {
				return err
			}
			delposs[i] = pos
		}*/

	err := f.removeInternal(dels)
	if err != nil {
		return err
	}
	err = f.addInternal(adds)
	if err != nil {
		return err
	}

	return f.reHash()
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
