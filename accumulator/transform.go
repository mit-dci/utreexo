package accumulator

// remTrans returns a slice arrow in bottom row to top row.
// also returns all "dirty" positions which need to be hashed after the swaps
func remTrans2(dels []uint64, numLeaves uint64, forestRows uint8) [][]arrow {
	// calculate the number of leaves after deletion
	nextNumLeaves := numLeaves - uint64(len(dels))

	// Initialize swaps and collapses. Swaps are where a leaf should go
	// collapses are for the roots
	swaps := make([][]arrow, forestRows)
	// a bit ugly: collapses also [][], but only have 1 or 0 things per row
	collapses := make([][]arrow, forestRows)

	// per forest row: 4 operations are executed
	// sort / extract / swap / root / promote
	for r := uint8(0); r < forestRows; r++ {
		// set current row slice as nil
		swaps[r] = nil

		// if there's nothing to delete, we're done
		if len(dels) == 0 {
			break
		}

		var twinNextDels []uint64

		// *** Delroot
		// TODO would be more elegant not to have this here.  But
		// easier to just delete the root first...

		// Check for root
		rootPresent := numLeaves&(1<<r) != 0
		rootPos := rootPosition(numLeaves, r, forestRows)

		// delRoot deletes all the roots of a given tree
		// TODO would be more elegant not to have this here. But
		// easier to just delete the root first...
		if rootPresent && dels[len(dels)-1] == rootPos {
			dels = dels[:len(dels)-1] // pop off the last del
			rootPresent = false
		}
		delRemains := len(dels)%2 != 0

		// *** Twin
		twinNextDels, dels = extractTwins(dels, forestRows)
		swaps[r] = makeSwaps(dels, delRemains, rootPresent, rootPos)
		collapses[r] = makeCollapse(dels, delRemains, rootPresent, r, numLeaves, nextNumLeaves, forestRows)

		// done with this row, move dels and proceed up to next row
		swapNextDels := makeSwapNextDels(dels, delRemains, rootPresent, forestRows)
		dels = mergeSortedSlices(twinNextDels, swapNextDels)
	}
	swapCollapses(swaps, collapses, forestRows)

	// merge slice of collapses, placing the collapses at the end of the row
	// ... is that the right place to put them....?
	for i, c := range collapses {
		if len(c) == 1 && c[0].from != c[0].to {
			swaps[i] = append(swaps[i], c[0])
		}
	}

	return swaps
}

// makeCollapse determines whether a collapse should take place given a list
// of nodes to delete and misc info. If the collapse is present, it consists
// of an single arrow (a source index and a destination index).
//
// A collapse is a deferred swap, that swaps a root or the sibling of a
// leftover deletion to its new position in the modified tree. A collapse can
// not directly be appended to the regular swaps because the collapse
// destination changes with upper row swaps. swapCollapses turns the collapse
// into a regular swap by computing a new destination position so that after
// upper row swaps the node is located at the collapse destination.
//
// delRemains must be true if dels has an odd length.
// r is the row number to generate a collapse for, 0 is the leaf level.
// rootPresent must be true if there is a tree root in the row r.
// nextNumLeaves is the final amount of leaves, after the whole deletion
// process. Note that if r!=0, some dels may already have happened, the slice
// will be shorter.
//
// Example using 15-leaf tree in printout.txt line 21: (forestRows=4, numLeaves=15)
//
// Let's say we are deleting nodes 4 through 9 (6 nodes, so delRemains=False).
//
// For this example, let's take the call to this function
// that happens when r=0 (bottom row).
//
// In this case, makeCollapse will be called with dels=[18,19] (from extractTwins),
// since those are parents of twins (in the row above) that will be deleted.
//
// rootPresent will be True, and the list of deletions has an even length,
// so the first switch branch below will be used.
//
// Since we are processing the bottom row, nextNumLeaves will be
// numLeaves-length(dels) = 15-6 = 9,
// because the dels passed to this function from remTrans2 hasn't been cut yet.
//
// So the root source position will be
// rootPosition(nextNumLeaves=15, row=0, height=4) = 14,
// and the root destination position will be
// rootPosition(numLeaves=9, row=0, height=4) = 8.
//
// It is a collapse and not a swap because the position 8 is going to be deleted,
// and a swap will write into that position.
func makeCollapse(dels []uint64, delRemains, rootPresent bool, r uint8, numLeaves, nextNumLeaves uint64, forestRows uint8) []arrow {
	rootDest := rootPosition(nextNumLeaves, r, forestRows)
	switch {
	// root but no del, and del but no root
	// these are special cases, need to run collapseCheck
	// on the collapses with later rows of swaps
	case !delRemains && rootPresent:
		rootSrc := rootPosition(numLeaves, r, forestRows)
		return []arrow{{from: rootSrc, to: rootDest}}
	case delRemains && !rootPresent:
		// no root but 1 del: sibling becomes root & collapses
		// in this case, mark as deleted
		rootSrc := dels[len(dels)-1] ^ 1
		return []arrow{{from: rootSrc, to: rootDest}}
	default:
		return nil
	}
}

func makeSwapNextDels(dels []uint64, delRemains, rootPresent bool, forestRows uint8) []uint64 {
	numSwaps := len(dels) >> 1
	if delRemains && !rootPresent {
		numSwaps++
	}
	swapNextDels := make([]uint64, numSwaps)
	i := 0
	for ; len(dels) > 1; dels = dels[2:] {
		swapNextDels[i] = parent(dels[1], forestRows)
		i++
	}
	if delRemains && !rootPresent {
		// deletion promotes to next row
		swapNextDels[i] = parent(dels[0], forestRows)
	}
	return swapNextDels
}

func makeSwaps(dels []uint64, delRemains, rootPresent bool, rootPos uint64) []arrow {
	numSwaps := len(dels) >> 1
	if delRemains && rootPresent {
		numSwaps++
	}
	rowSwaps := make([]arrow, numSwaps)
	// *** swap
	i := 0
	for ; len(dels) > 1; dels = dels[2:] {
		rowSwaps[i] = arrow{from: dels[1] ^ 1, to: dels[0]}
		i++
	}
	// *** root
	if delRemains && rootPresent {
		// root to del, no collapse / upper del
		rowSwaps[i] = arrow{from: rootPos, to: dels[0]}
	}
	return rowSwaps
}

func swapInRow(s arrow, collapses [][]arrow, r uint8, forestRows uint8) {
	for cr := uint8(0); cr < r; cr++ {
		if len(collapses[cr]) == 0 {
			continue
		}
		mask := swapIfDescendant(s, collapses[cr][0], r, cr, forestRows)
		collapses[cr][0].to ^= mask
	}
}

// swapCollapses applies all swaps to lower collapses.
func swapCollapses(swaps, collapses [][]arrow, forestRows uint8) {
	// If there is nothing to collapse, we're done
	if len(collapses) == 0 {
		return
	}

	// For all the collapses, go through all of them except for the root
	for r := uint8(len(collapses)) - 1; r != 0; r-- {
		// go through through swaps at this row
		for _, s := range swaps[r] {
			swapInRow(s, collapses, r, forestRows)
		}

		if len(collapses[r]) == 0 {
			continue
		}
		// exists / non-nil; affect lower collapses
		rowcol := collapses[r][0]
		// do collapse on lower collapses
		swapInRow(rowcol, collapses, r, forestRows)
	}
}

// swapIfDescendant checks if a.to or a.from is above b
// if a.to xor a.from is above b, it will also calculates the new position of b
// were it swapped to being below the other one.  Returns what to xor b.to.
func swapIfDescendant(a, b arrow, ar, br, forestRows uint8) (subMask uint64) {
	// ar= row of a, br= row of b, fr= forest row
	hdiff := ar - br
	// a must always be higher than b; we're not checking for that
	// TODO probably doesn't matter, but it's running upMany every time
	// isAncestorSwap is called.  UpMany isn't even a loop so who cares.  But
	// could inline that up to what calls this and have bup as an arg..?
	bup := parentMany(b.to, hdiff, forestRows)
	if (bup == a.from) != (bup == a.to) {
		// b.to is below one but not both, swap it
		rootMask := a.from ^ a.to
		subMask = rootMask << hdiff
		// fmt.Printf("collapse %d->%d to %d->%d because of %v\n",
		// b.from, b.to, b.from, b.to^subMask, a)
	}

	return subMask
}

// FloorTransform calls remTrans2 and expands it to give all leaf swaps
// TODO optimization: if children move, parents don't need to move.
// (But siblings might)
func floorTransform(
	dels []uint64, numLeaves uint64, forestRows uint8) []arrow {
	// fmt.Printf("(undo) call remTr %v nl %d fr %d\n", dels, numLeaves, forestRows)
	swaprows := remTrans2(dels, numLeaves, forestRows)
	// fmt.Printf("td output %v\n", swaprows)
	var floor []arrow
	for r, row := range swaprows {
		for _, a := range row {
			if a.from == a.to {
				continue
				// TODO: why do these even exist?  get rid of them from
				// removeTransform output?
			}
			leaves := a.toLeaves(uint8(r), forestRows)
			floor = append(floor, leaves...)
		}
	}
	return floor
}
