package utreexo

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
	// a bit ugly: collapses also [][], but only have 1 or 0 things per row
	collapses := make([][]arrow, fHeight)
	// fmt.Printf("rt2 on %v\n", dels)
	// per row: sort / extract / swap / root / promote
	for h := uint8(0); h < fHeight; h++ {
		// start with nil swap slice, not a single {0, 0}
		swaps[h] = nil
		// fmt.Printf("del %v col %v h %d\n", dels, collapses, h)
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
			// fmt.Printf("deleting root %d\n", rootPos)
			dels = dels[:len(dels)-1] // pop off the last del
			rootPresent = false
		}
		delRemains := len(dels)%2 != 0

		// *** dedupe
		twinNextDels, dels = ExTwin2(dels, fHeight)
		// fmt.Printf("tnd %v dels %v\n", twinNextDels, dels)
		// *** swap
		for len(dels) > 1 {
			swaps[h] = append(swaps[h],
				arrow{from: dels[1] ^ 1, to: dels[0]})
			// deletion promotes to next row
			swapNextDels = append(swapNextDels, up1(dels[1], fHeight))
			dels = dels[2:]
		}

		// *** root
		if rootPresent && delRemains { // root to del, no collapse / upper del
			swaps[h] = append(swaps[h], arrow{from: rootPos, to: dels[0]})
		}

		// root but no del, and del but no root
		// these are special cases, need to run collapseCheck
		// on the collapses with later rows of swaps
		if rootPresent && !delRemains { // stash root (collapses)
			rootSrc := topPos(numLeaves, h, fHeight)
			rootDest := topPos(nextNumLeaves, h, fHeight)
			collapses[h] = []arrow{arrow{from: rootSrc, to: rootDest}}
			// fmt.Printf("%d root, collapse to %d\n", rootSrc, rootDest)
		}
		// no root but 1 del: sibling becomes root & collapses
		// in this case, mark as deleted
		if !rootPresent && delRemains {
			rootSrc := dels[0] ^ 1
			rootDest := topPos(nextNumLeaves, h, fHeight)
			collapses[h] = []arrow{arrow{from: rootSrc, to: rootDest}}
			// fmt.Printf("%d promote to root, collapse to %d\n", rootSrc, rootDest)
			swapNextDels = append(swapNextDels, up1(dels[0], fHeight))
		}

		// if neither haveDel nor rootPresent, nothing to do.
		// done with this row, move dels and proceed up to next row
		dels = mergeSortedSlices(twinNextDels, swapNextDels)
	}
	// fmt.Printf("swaps %v\n", swaps)
	// fmt.Printf("collapses: %v\n", collapses)
	swapCollapses(swaps, collapses, fHeight)
	// fmt.Printf("rt2 swaps %v collapses %v\n", swaps, collapses)

	// merge slice of collapses, placing the collapses at the end of the row
	// ... is that the right place to put them....?
	for i, c := range collapses {
		if len(c) == 1 && c[0].from != c[0].to {
			swaps[i] = append(swaps[i], c[0])
		}
	}
	// topUp(fHeight)
	return swaps
}

// swapCollapses applies all swaps to lower collapses.
func swapCollapses(swaps, collapses [][]arrow, fh uint8) {
	if len(collapses) == 0 {
		return
	}
	for h := uint8(len(collapses)) - 1; h != 0; h-- {
		// go through through swaps at this height
		// fmt.Printf("h %d swaps %v\n", h, swaps)
		for _, s := range swaps[h] {
			for ch := uint8(0); ch < h; ch++ {
				if len(collapses[ch]) == 0 {
					continue
				}
				mask := swapIfDescendant(s, collapses[ch][0], h, ch, fh)
				if mask != 0 {
					// fmt.Printf("****col %v becomes ", c)
					collapses[ch][0].to ^= mask
					// fmt.Printf("%v due to %v\n", collapses[ch], s)
				}
			}
		}

		if len(collapses[h]) == 0 {
			continue
		}
		// exists / non-nil; affect lower collapses
		rowcol := collapses[h][0]
		// do collapse on lower collapses
		for ch := uint8(0); ch < h; ch++ {
			if len(collapses[ch]) == 0 {
				continue
			}
			mask := swapIfDescendant(rowcol, collapses[ch][0], h, ch, fh)
			if mask != 0 {
				// fmt.Printf("****col %v becomes ", collapses[ch])
				collapses[ch][0].to ^= mask
				// fmt.Printf("%v due to %v\n", collapses[ch], rowcol)
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
		// fmt.Printf("collapse %d->%d to %d->%d because of %v\n",
		// b.from, b.to, b.from, b.to^subMask, a)

	}
	return subMask
}

// TODO optimization: if children move, parents don't need to move.
// (But siblings might)

// floorTransform calles remTrans2 and expands it to give all leaf swaps
func floorTransform(
	dels []uint64, numLeaves uint64, fHeight uint8) []arrow {
	// fmt.Printf("(undo) call remTr %v nl %d fh %d\n", dels, numLeaves, fHeight)
	swaprows := remTrans2(dels, numLeaves, fHeight)
	// fmt.Printf("td output %v\n", swaprows)
	var floor []arrow
	for h, row := range swaprows {
		for _, a := range row {
			if a.from == a.to {
				continue
				// TODO: why do these even exist?  get rid of them from
				// removeTransform output?
			}
			leaves := a.toLeaves(uint8(h), fHeight)
			for _, l := range leaves {
				floor = append(floor, l)
			}
		}
	}
	return floor
}
