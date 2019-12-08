package utreexo

import (
	"fmt"
)

/*
The transform operations can probably be moved into a different package even.
They're some of the tricky parts of utreexo, on how to rearrange the forest nodes
when deletions occur.
*/

/*
idea for transform
get rid of stash, and use swaps instead.
Where you would encounter stashing, here's what to do:
stash in place: It's OK, that doesn't even count as a stash
stash to sibling: Also OK, go for it.  The sib must have been deleted, right?

stash elsewhere: Only swap to the LSB of destination (sibling).  If LSB of
destination is same as current LSB, don't move.  You will get there later.
When you do this, you still flag the parent as "deleted" even though it's still
half-there.

Maybe modify removeTransform to do this; that might make leaftransform easier
*/

// remTrans2 -- simpler and better -- lets see if it works!
// returns a slice of swapswithHeight in bottom to top order
// also returns all "dirty" positions which need to be hashed after the swaps

func remTrans2(
	dels []uint64, numLeaves uint64, fHeight uint8) [][]arrow {
	nextNumLeaves := numLeaves - uint64(len(dels))
	// fHeight := treeHeight(numLeaves)
	// var swaps, collapses []arrowh
	swaps := make([][]arrow, fHeight)
	collapses := make([]arrow, fHeight)
	fmt.Printf("rt2 on %v\n", dels)
	// per row: sort / extract / swap / root / promote
	for h := uint8(0); h < fHeight; h++ {
		fmt.Printf("h %d del %v col %v\n", h, dels, collapses)
		if len(dels) == 0 { // if there's nothing to delete, we're done
			break
		}
		var twinNextDels, swapNextDels []uint64
		rootPresent := numLeaves&(1<<h) != 0
		rootPos := topPos(numLeaves, h, fHeight)

		// *** delroot
		// TODO would be more elegant not to have this here.  But
		// easier to just delete the root first...
		if rootPresent && dels[len(dels)-1] == rootPos {
			fmt.Printf("deleting root %d\n", rootPos)
			dels = dels[:len(dels)-1] // pop off the last del
			rootPresent = false
		}
		delRemains := len(dels)%2 != 0

		// *** dedupe
		twinNextDels, dels = ExTwin2(dels, fHeight)

		// *** swap
		for len(dels) > 1 {
			swaps[h] = append(swaps[h],
				arrow{from: dels[1] ^ 1, to: dels[0]})
			// deletion promotes to next row
			swapNextDels = append(swapNextDels, up1(dels[1], fHeight))
			dels = dels[2:]
		}

		// *** root
		if rootPresent && delRemains { // root to del, no stash / upper del
			swaps[h] = append(swaps[h], arrow{from: rootPos, to: dels[0]})
		}

		// root but no del, and del but no root
		// these are special cases, need to run collapseCheck
		// on the collapses with later rows of swaps
		if rootPresent && !delRemains { // stash root (collapses)
			rootSrc := topPos(numLeaves, h, fHeight)
			rootDest := topPos(nextNumLeaves, h, fHeight)
			collapses[h] = arrow{from: rootSrc, to: rootDest}
			fmt.Printf("%d root, collapse to %d\n", rootSrc, rootDest)
		}
		// no root but 1 del: sibling becomes root & collapses
		// in this case, mark as deleted
		if !rootPresent && delRemains {
			rootSrc := dels[0] ^ 1
			rootDest := topPos(nextNumLeaves, h, fHeight)
			collapses[h] = arrow{from: rootSrc, to: rootDest}
			fmt.Printf("%d promote to root, collapse to %d\n", rootSrc, rootDest)
			swapNextDels = append(swapNextDels, up1(dels[0], fHeight))
		}
		// if neither haveDel nor rootPresent, nothing to do
		// done with this row, move dels and proceed up to next row
		dels = mergeSortedSlices(twinNextDels, swapNextDels)
	}

	swapCollapses(swaps, collapses, fHeight)
	fmt.Printf("rt2 swaps %v collapses %v\n", swaps, collapses)

	// merge slice of collapses, placing the collapses at the end of the row
	// ... is that the right place to put them....?
	for i, c := range collapses {
		swaps[i] = append(swaps[i], c)
	}

	// topUp(fHeight)
	return swaps
}

func swapCollapses(swaps [][]arrow, collapses []arrow, fh uint8) {
	for h := uint8(len(collapses)) - 1; h != 255; h-- {
		// go through through swaps at this height
		fmt.Printf("h %d swaps %v\n", h, swaps)
		for _, s := range swaps[h] {
			for ch := uint8(0); ch < h; ch++ {
				c := collapses[ch]
				mask := swapIfDescendant(s, c, h, ch, fh)
				if mask != 0 {
					fmt.Printf("****col %v becomes ", c)
					collapses[ch].to ^= mask
					fmt.Printf("%v due to %v\n", collapses[ch].to, s)
				}
			}
		}

		if collapses[h].to != collapses[h].from { // exists / non-nil
			rowcol := collapses[h]
			// do collapse on lower collapses
			for ch := uint8(0); ch < h; ch++ {
				c := collapses[ch]
				mask := swapIfDescendant(rowcol, c, h, ch, fh)
				if mask != 0 {
					fmt.Printf("****col %v becomes ", c)
					collapses[ch].to ^= mask
					fmt.Printf("%v due to %v\n", c, collapses[ch])
				}
			}
		}
	}
}

// swapIfDescendant checks if a.to or a.from is above b
// ah= height of a, bh=height of b, fh= forest height
// if a.to xor a.from is above b, it will also calculates the new position of b
// were it swapped to being below the other one.  Returns what to xor b.to.
func swapIfDescendant(a, b arrow, ah, bh, fh uint8) (subMask uint64) {
	hdiff := ah - bh
	// a must always be higher than b; we're not checking for that
	// TODO probably doesn't matter, but it's running upMany every time
	// isAncestorSwap is called.  UpMany isn't even a loop so who cares.  But
	// could inline that up to what calls this and have bup as an arg..?
	bup := upMany(b.to, hdiff, fh)
	if (bup == a.from) != (bup == a.to) {
		// b.to is below one but not both, swap it
		topMask := a.from ^ a.to
		subMask = topMask << hdiff
		fmt.Printf("collapse %d->%d to %d->%d because of %v\n",
			b.from, b.to, b.from, b.to^subMask, a)

	}
	return subMask
}

// RemoveTransform takes in the positions of the leaves to be deleted, as well
// as the number of leaves and height of the forest (semi redundant).  It returns
// 2 slices of movePos which is a sequential list of from/to move pairs.

// Stashes always move to roots; for pollard this can move directly to a root.

// This list is "raw" and a higher level move *implies* moving the whole subtree.
// to get to a direct from/to mapping on the whole tree level, you will need
// to process the movePos
func removeTransformx(
	dels []uint64, numLeaves uint64, fHeight uint8) ([]arrow, []arrow) {

	// note that RemoveTransform is still way sub-optimal in that I'm sure
	// you can do this directly without the n*log(n) subtree moving...

	topPoss, _ := getTopsReverse(numLeaves, fHeight)
	nextTopPoss, _ := getTopsReverse(numLeaves-uint64(len(dels)), fHeight)

	// m is the main list of arrows
	var a, stash []arrow
	// stash is a list of nodes to move later.  They end up as tops.
	// stash := make([]move, len(nextTopPoss))
	// for i, _ := range stash {
	// 	stash[i].to = nextTopPoss[i]
	// }

	var up1DelSlice []uint64 // the next ds, one row up (in construction)

	// the main floor loop.
	// per row: sort / extract / swap / root / promote
	for h := uint8(0); h <= fHeight; h++ {
		if len(dels) == 0 {
			break
		}

		// *** sort.  Probably pointless on upper floors..?
		// apparently it's not pointless at all which is somewhat surprising.
		// maybe I should figure out why, or change the appending so that
		// everything put in is already sorted...
		sortUint64s(dels)

		// check for root deletion (it can only be the last one)
		// there should always be a topPoss remaining...
		if len(topPoss) > 0 && topPoss[0] == dels[len(dels)-1] {
			dels = dels[:len(dels)-1] // pop off the end
			topPoss = topPoss[1:]     // pop off the top
		}

		// *** extract / dedupe
		var twins []uint64
		twins, dels = ExtractTwins(dels)
		// run through the slice of deletions, and 'dedupe' by eliminating siblings
		for _, sib := range twins {
			up1DelSlice = append(up1DelSlice, up1(sib, fHeight))
		}

		// *** swap
		for len(dels) > 1 {

			if sibSwap && dels[0]&1 == 0 { // if destination is even (left)
				a = append(a, arrow{from: dels[0] ^ 1, to: dels[0]})
				// fmt.Printf("swap %d -> %d\n", m[len(m)-1].from, m[len(m)-1].to)
				// set destination to newly vacated right sibling
				dels[0] = dels[0] ^ 1
			}

			a = append(a, arrow{from: dels[1] ^ 1, to: dels[0]})
			// fmt.Printf("swap %d -> %d\n", m[len(m)-1].from, m[len(m)-1].to)

			// deletion promotes to next row
			up1DelSlice = append(up1DelSlice, up1(dels[1], fHeight))
			dels = dels[2:]
		}

		// *** root
		// If we're deleting it, delete it now; its presence is important for
		// subsequent swaps
		// scenarios: deletion is present / absent, and root is present / absent
		var rootPos uint64
		var rootPresent bool
		// check if a top is present on this floor
		if len(topPoss) > 0 && detectHeight(topPoss[0], fHeight) == h {
			rootPos = topPoss[0]
			rootPresent = true
			// if it was present, pop it off
			topPoss = topPoss[1:]
		}

		// the last remaining deletion (if exists) can swap with the root

		// weve already deleted roots either in the delete phase, so there can't
		// be a root here that we are deleting.
		var delPos uint64
		var haveDel bool
		if len(dels) == 1 {
			delPos = dels[0]
			haveDel = true
		}
		fmt.Printf("h %d haveDel %v rootpresent %v\n", h, haveDel, rootPresent)

		if haveDel && rootPresent {
			// deroot, move to sibling
			if sibSwap && delPos&1 == 0 { // if destination is even (left)
				a = append(a, arrow{from: delPos ^ 1, to: delPos})
				// set destinationg to newly vacated right sibling
				delPos = delPos ^ 1 // |1 should also work
			}
			a = append(a, arrow{from: rootPos, to: delPos})
		}

		if haveDel != rootPresent && len(nextTopPoss) == 0 {
			panic("nextTopPos nil")
		}

		if haveDel && !rootPresent {
			// stash sibling
			// if delPos^1 != nextTopPoss[0] {
			stash = append(stash, arrow{from: delPos ^ 1, to: nextTopPoss[0]})
			// }

			nextTopPoss = nextTopPoss[1:]
			// mark parent for deletion. this happens even if the node
			// being promoted to root doesn't move
			up1DelSlice = append(up1DelSlice, up1(delPos, fHeight))
		}

		if !haveDel && rootPresent {
			//  stash existing root (it will collapse left later)
			stash = append(stash, arrow{from: rootPos, to: nextTopPoss[0]})
			nextTopPoss = nextTopPoss[1:]
		}

		// done with one row, set ds to the next slice (promote)
		dels = up1DelSlice
		up1DelSlice = []uint64{}
	}
	if len(dels) != 0 {
		fmt.Printf("finished deletion climb but %d deletion left %v",
			len(dels), dels)
		return nil, nil
	}

	return stash, a
}

// TODO optimization: if children move, parents don't need to move.
// (But siblings might)

// floorTransform calles remTrans2 and expands it to give all leaf swaps
func floorTransform(
	dels []uint64, numLeaves uint64, fHeight uint8) []arrow {
	fmt.Printf("(undo) call remTr %v nl %d fh %d\n", dels, numLeaves, fHeight)
	swaprows := remTrans2(dels, numLeaves, fHeight)
	fmt.Printf("td output %v\n", swaprows)

	var floor []arrow

	fmt.Printf("raw: ")
	for h, row := range swaprows {
		for _, a := range row {
			fmt.Printf("%d -> %d\t", a.from, a.to)
			if a.from == a.to {
				fmt.Printf("omitting ################# %d -> %d\n", a.to, a.to)
				continue
				// TODO: why do these even exist?  get rid of them from
				// removeTransform output?
			}
			leaves := a.toLeaves(uint8(h), fHeight)
			fmt.Printf(" leaf: ")

			for _, l := range leaves {
				fmt.Printf("%d -> %d\t", l.from, l.to)
				floor = append(floor, l)
				// can cutthrough work..?

				// prevTo, ok := arMap[l.to]
				// if  ok {
				// fmt.Printf("%d in map\n", l.to)
				// arMap[l.from] = prevTo
				// delete(arMap, l.to)
				// } else {
				// arMap[l.from] = l.to
				// }
			}
			fmt.Printf("\n")
		}
	}

	fmt.Printf("floor: %v\n", floor)

	return floor
}
