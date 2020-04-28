package accumulator

import (
	"github.com/mit-dci/utreexo/accumulator/util"
)

// RemTrans returns a slice Arrow in bottom row to top row.
// also returns all "dirty" positions which need to be hashed after the swaps
func RemTrans(dels []uint64, numLeaves uint64, fHeight uint8) [][]util.Arrow {
	// calculate the number of leaves after deletion
	nextNumLeaves := numLeaves - uint64(len(dels))

	// Initialize swaps and collapses. Swaps are where a leaf should go
	// collapses are for the roots
	swaps := make([][]util.Arrow, fHeight)
	// a bit ugly: collapses also [][], but only have 1 or 0 things per row
	collapses := make([][]util.Arrow, fHeight)

	// per forest row (aka height): 4 operations are executed
	// sort / extract / swap / root / promote
	for h := uint8(0); h < fHeight; h++ {
		// set current height slice as nil
		swaps[h] = nil

		// if there's nothing to delete, we're done
		if len(dels) == 0 {
			break
		}

		var twinNextDels, swapNextDels []uint64

		// *** Delroot
		// TODO would be more elegant not to have this here.  But
		// easier to just delete the root first...

		// Check for root
		rootPresent := util.CheckForRoot(numLeaves, h)
		rootPos := util.TopPos(numLeaves, h, fHeight)

		if rootPresent && dels[len(dels)-1] == rootPos {
			dels = dels[:len(dels)-1] // pop off the last del
			rootPresent = false
		}
		delRemains := len(dels)%2 != 0

		// *** Twin
		twinNextDels, dels = util.ExTwin2(dels, fHeight)

		// *** swap
		for len(dels) > 1 {
			swaps[h] = append(swaps[h],
				util.Arrow{From: dels[1] ^ 1, To: dels[0]})
			// deletion promotes to next row
			swapNextDels = append(swapNextDels, util.Up1(dels[1], fHeight))
			dels = dels[2:]
		}

		// *** root
		if rootPresent && delRemains { // root to del, no collapse / upper del
			swaps[h] = append(swaps[h], util.Arrow{From: rootPos, To: dels[0]})
		}

		// root but no del, and del but no root
		// these are special cases, need to run collapseCheck
		// on the collapses with later rows of swaps
		if rootPresent && !delRemains { // stash root (collapses)
			rootSrc := util.TopPos(numLeaves, h, fHeight)
			rootDest := util.TopPos(nextNumLeaves, h, fHeight)
			collapses[h] = []util.Arrow{util.Arrow{From: rootSrc, To: rootDest}}
		}
		// no root but 1 del: sibling becomes root & collapses
		// in this case, mark as deleted
		if !rootPresent && delRemains {
			rootSrc := dels[0] ^ 1
			rootDest := util.TopPos(nextNumLeaves, h, fHeight)
			collapses[h] = []util.Arrow{util.Arrow{From: rootSrc, To: rootDest}}

			swapNextDels = append(swapNextDels, util.Up1(dels[0], fHeight))
		}

		// if neither haveDel nor rootPresent, nothing to do.
		// done with this row, move dels and proceed up to next row
		dels = util.MergeSortedSlices(twinNextDels, swapNextDels)
	}
	swapCollapses(swaps, collapses, fHeight)

	// merge slice of collapses, placing the collapses at the end of the row
	// ... is that the right place to put them....?
	for i, c := range collapses {
		if len(c) == 1 && c[0].From != c[0].To {
			swaps[i] = append(swaps[i], c[0])
		}
	}

	return swaps
}

// swapCollapses applies all swaps to lower collapses.
func swapCollapses(swaps, collapses [][]util.Arrow, fh uint8) {
	// If there is nothing to collapse, we're done
	if len(collapses) == 0 {
		return
	}

	// For all the collapses, go through all of them except for the root
	for h := uint8(len(collapses)) - 1; h != 0; h-- {
		// go through through swaps at this height
		for _, s := range swaps[h] {
			for ch := uint8(0); ch < h; ch++ {
				if len(collapses[ch]) == 0 {
					continue
				}
				mask := swapIfDescendant(s, collapses[ch][0], h, ch, fh)
				if mask != 0 {
					// fmt.Printf("****col %v becomes ", c)
					collapses[ch][0].To ^= mask
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
				collapses[ch][0].To ^= mask
				// fmt.Printf("%v due to %v\n", collapses[ch], rowcol)
			}
		}
	}
}

// delRoot deletes all the roots of a given tree
// TODO would be more elegant not to have this here. But
// easier to just delete the root first...
func delRoot(dels []uint64, rootPresent bool, rootPos uint64) ([]uint64, bool) {
	if rootPresent && dels[len(dels)-1] == rootPos {
		dels = dels[:len(dels)-1] // pop off the last del(aka root)
		rootPresent = false
	}

	return dels, rootPresent
}

// swapIfDescendant checks if a.to or a.from is above b
// if a.to xor a.from is above b, it will also calculates the new position of b
// were it swapped to being below the other one.  Returns what to xor b.to.
func swapIfDescendant(a, b util.Arrow, ah, bh, fh uint8) (subMask uint64) {
	// ah= height of a, bh=height of b, fh= forest height
	hdiff := ah - bh
	// a must always be higher than b; we're not checking for that
	// TODO probably doesn't matter, but it's running upMany every time
	// isAncestorSwap is called.  UpMany isn't even a loop so who cares.  But
	// could inline that up to what calls this and have bup as an arg..?
	bup := util.UpMany(b.To, hdiff, fh)
	if (bup == a.From) != (bup == a.To) {
		// b.To is below one but not both, swap it
		TopMask := a.From ^ a.To
		subMask = TopMask << hdiff
		// fmt.Printf("collapse %d->%d to %d->%d because of %v\n",
		// b.from, b.to, b.from, b.to^subMask, a)
	}

	return subMask
}

// FloorTransform calls remTrans2 and expands it to give all leaf swaps
// TODO optimization: if children move, parents don't need to move.
// (But siblings might)
func FloorTransform(
	dels []uint64, numLeaves uint64, fHeight uint8) []util.Arrow {
	// fmt.Printf("(undo) call remTr %v nl %d fh %d\n", dels, numLeaves, fHeight)
	swaprows := RemTrans(dels, numLeaves, fHeight)
	// fmt.Printf("td output %v\n", swaprows)
	var floor []util.Arrow
	for h, row := range swaprows {
		for _, a := range row {
			if a.From == a.To {
				continue
				// TODO: why do these even exist?  get rid of them from
				// removeTransform output?
			}
			leaves := a.ToLeaves(uint8(h), fHeight)
			for _, l := range leaves {
				floor = append(floor, l)
			}
		}
	}
	return floor
}
