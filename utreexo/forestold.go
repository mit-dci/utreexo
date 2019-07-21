package utreexo

import (
	"fmt"
	"time"
)

// ---------------------------   old stuff, to delete when possible

// Deletion and addition are ~completely separate operations in the forest,
// so lets break em into different methods.

// Remove deletes nodes from the forest.  This is the hard part :)
// note that in the Forest method, we don't actually use the proofs,
// just the position being deleted

func (f *Forest) removeInternal(dels []uint64) ([]undo, error) {
	starttime := time.Now()
	if len(dels) == 0 {
		return nil, nil
	}

	// clear out undo slice; will be written to in this function
	f.currentUndo = []undo{}

	numDeletions := uint64(len(dels))
	if numDeletions > f.numLeaves {
		return nil, fmt.Errorf(
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
			return nil, fmt.Errorf(
				"Trying to delete leaf at %d, beyond max %d", dpos, f.numLeaves)
		}
		//		ds = append(ds, dpos)
		// have the forest so no need to populate with d.siblings

		// clear all entries from positionMap as they won't be needed any more
		// quick & easy; doesn't affcet the tree
		delete(f.positionMap, f.forest[dpos].Mini())

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

	// what happens if you delete everything? do we care?  probably not

	// TODO need to call transform for forest.Modify.  Seriously it's the same
	// code twice.

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
				err := f.moveSubtree(move{dels[0] ^ 1, dels[0]}) // swap siblings first
				// TODO do I need to pass dirtymap there? I think so, but not sure
				if err != nil {
					return nil, err
				}
				// set destinationg to newly vacated right sibling
				dels[0] = dels[0] ^ 1
			}

			err := f.moveSubtree(move{dels[1] ^ 1, dels[0]})
			if err != nil {
				return nil, err
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
			return nil, fmt.Errorf("%d deletions in root phase\n", len(dels))
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
			return nil, err
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
		return nil, fmt.Errorf("finished deletion climb but %d deletion left", len(dels))
	}

	// move subtrees from the stash to where they should go
	for height, stash := range stashMap {
		destPos := nextRootPosMap[height]
		//		fmt.Printf("moving stashed subtree to h %d pos %d\n", height, destPos)

		err := f.writeSubtree(stash, destPos)
		if err != nil {
			return nil, err
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

	return f.currentUndo, nil
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
			err := f.moveSubtree(move{delPos ^ 1, delPos}) // swap siblings first
			if err != nil {
				return 0, stash, err
			}
			// set destinationg to newly vacated right sibling
			delPos = delPos ^ 1 // |1 should also work
		}

		err := f.moveSubtree(move{rootPos, delPos})
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

	//	fmt.Printf("moved position %d to h %d rooat stash\n", stashPos, h)
	return upDel, stash, nil
}
