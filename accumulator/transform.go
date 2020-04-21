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

		var twinNextDels, swapNextDels []uint64

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

		// *** swap
		for len(dels) > 1 {
			swaps[r] = append(swaps[r],
				arrow{from: dels[1] ^ 1, to: dels[0]})
			// deletion promotes to next row
			swapNextDels = append(swapNextDels, parent(dels[1], forestRows))
			dels = dels[2:]
		}

		// *** root
		if rootPresent && delRemains { // root to del, no collapse / upper del
			swaps[r] = append(swaps[r], arrow{from: rootPos, to: dels[0]})
		}

		// root but no del, and del but no root
		// these are special cases, need to run collapseCheck
		// on the collapses with later rows of swaps
		if rootPresent && !delRemains { // stash root (collapses)
			rootSrc := rootPosition(numLeaves, r, forestRows)
			rootDest := rootPosition(nextNumLeaves, r, forestRows)
			collapses[r] = []arrow{arrow{from: rootSrc, to: rootDest}}
		}
		// no root but 1 del: sibling becomes root & collapses
		// in this case, mark as deleted
		if !rootPresent && delRemains {
			rootSrc := dels[0] ^ 1
			rootDest := rootPosition(nextNumLeaves, r, forestRows)
			collapses[r] = []arrow{arrow{from: rootSrc, to: rootDest}}

			swapNextDels = append(swapNextDels, parent(dels[0], forestRows))
		}

		// if neither haveDel nor rootPresent, nothing to do.
		// done with this row, move dels and proceed up to next row
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
			for cr := uint8(0); cr < r; cr++ {
				if len(collapses[cr]) == 0 {
					continue
				}
				mask := swapIfDescendant(s, collapses[cr][0], r, cr, forestRows)
				if mask != 0 {
					// fmt.Printf("****col %v becomes ", c)
					collapses[cr][0].to ^= mask
					// fmt.Printf("%v due to %v\n", collapses[cr], s)
				}
			}
		}

		if len(collapses[r]) == 0 {
			continue
		}
		// exists / non-nil; affect lower collapses
		rowcol := collapses[r][0]
		// do collapse on lower collapses
		for cr := uint8(0); cr < r; cr++ {
			if len(collapses[cr]) == 0 {
				continue
			}

			mask := swapIfDescendant(rowcol, collapses[cr][0], r, cr, forestRows)

			if mask != 0 {
				// fmt.Printf("****col %v becomes ", collapses[cr])
				collapses[cr][0].to ^= mask
				// fmt.Printf("%v due to %v\n", collapses[cr], rowcol)
			}
		}
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
			for _, l := range leaves {
				floor = append(floor, l)
			}
		}
	}
	return floor
}
