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

// IN PROGRESS
// OK there's still go to be some kind of "swap" idea, at least
// in the scope of this function.  I don't see any way to avoid that.
// But it does look like it can stay within the function.  If you have
// a stash, track it, and see when moves occur above it.  If they do,
// change the stash "to" to "what ends up there".
// e.g 4 leaves, delete 0.
// row 0: 1->2 stash
// row 1: 5->4 (stash but top so)
// 5 above 2; 2 becomes 0, row 0 move becomes 1->0.
// total output of function is {1,0} {5,4}

// remTrans2 -- simpler and better -- lets see if it works!
func remTrans2(dels []uint64, numLeaves uint64, fHeight uint8) []arrowh {
	nextNumLeaves := numLeaves - uint64(len(dels))
	// fHeight := treeHeight(numLeaves)
	var swaps, collapses []arrowh
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
			swaps = append(swaps,
				arrowh{from: dels[1] ^ 1, to: dels[0], ht: h})
			// deletion promotes to next row
			swapNextDels = append(swapNextDels, up1(dels[1], fHeight))
			dels = dels[2:]
		}

		// *** root
		if rootPresent && delRemains { // root to del, no stash / upper del
			swaps = append(swaps, arrowh{from: rootPos, to: dels[0], ht: h})
		}

		// root but no del, and del but no root
		// these are special cases, need to run collapseCheck
		// on the collapses with later rows of swaps
		if rootPresent && !delRemains { // stash root (collapses)
			rootSrc := topPos(numLeaves, h, fHeight)
			rootDest := topPos(nextNumLeaves, h, fHeight)
			collapses = append(collapses,
				arrowh{from: rootSrc, to: rootDest, ht: h})
			fmt.Printf("%d root, collapse to %d\n", rootSrc, rootDest)
		}
		// no root but 1 del: sibling becomes root & collapses
		// in this case, mark as deleted
		if !rootPresent && delRemains {
			rootSrc := dels[0] ^ 1
			rootDest := topPos(nextNumLeaves, h, fHeight)
			collapses = append(collapses,
				arrowh{from: rootSrc, to: rootDest, ht: h})
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
	si := 0
	for _, c := range collapses {
		for len(swaps) > si && swaps[si].ht <= c.ht {
			si++
		}
		swaps = append(swaps[:si], append([]arrowh{c}, swaps[si:]...)...)
	}

	return swaps
}

// swapCollapses modifies to field of arrows for root collapse
// rh = height of rowSwaps, fh = forest height
// go backwards in the slice, top down
func swapCollapses(swaps, collapses []arrowh, fh uint8) {
	if len(collapses) == 0 {
		return
	}
	fmt.Printf("swapCollapses on swaps %v collapses %v\n", swaps, collapses)

	// si, ci are indexes for swaps and collapses
	si, ci := len(swaps)-1, len(collapses)-1

	// go down from fh.  swaps at h0 can't modify anything so stop at 1
	// if si or ci get to -1 the
	for h := fh; h > 0; h-- {
		fmt.Printf("swapCol h %d\n", h)
		// tick through swaps at this height
		for si >= 0 && swaps[si].ht == h {
			// do swap on lower collapses
			for i, c := range collapses {
				fmt.Printf("swap %v on col %v\n", swaps[si], c)
				if c.ht < h {
					mask := swapIfDescendant(swaps[si], c, fh)
					if mask != 0 {
						fmt.Printf("****col %v becomes ", c)
						collapses[i].to ^= mask
						fmt.Printf("%v due to %v\n", c, collapses[ci])
					}
				}
			}
			si--
		}
		if ci >= 0 && collapses[ci].ht == h {
			// do collapse on lower collapses
			for i, c := range collapses {
				fmt.Printf("col %v on col %v\n", collapses[ci], c)
				if c.ht < h {
					mask := swapIfDescendant(collapses[ci], c, fh)
					if mask != 0 {
						fmt.Printf("****col %v becomes ", c)
						collapses[i].to ^= mask
						fmt.Printf("%v due to %v\n", c, collapses[ci])
					}
				}
			}
			ci--
		}
	}

}

// swapIfDescendant checks if a.to or a.from is above b
// ah= height of a, bh=height of b, fh= forest height
// if a.to xor a.from is above b, it will also calculates the new position of b
// were it swapped to being below the other one.  Returns what to xor b.to.
func swapIfDescendant(a, b arrowh, fh uint8) (subMask uint64) {
	hdiff := a.ht - b.ht
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
func removeTransform(
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

/*
removeTransform seems OK for pollard.  There might be ways to simplify but it's not
too bad.  For forest, however, removeTransform is a mess, requiring tons of code
afterwards, like get/write/moveSubTree.  But wee don't need those!
Here's what to build for a expanded / leaf / undo / forest transform.
Instead of moves & stashes at all rows, we only need to operate on the bottom row,
and the two types are moves and swaps.  Moves have a to and a from, where the element
at position from gets written to position to, and the element at position from is
deleted (it's no longer inside the numLeaves range).  Swaps are also 2 positions,
but there's no deletion, just a, b = b, a.
This is nicer because: 1) only operate on the bottom row; everything about gets
hashed into existence anyway.  2) only 1 element worth of extra memory is used.

... this might not make sense though because dirty bits are not as simple as things
that have moved.  If you move a big subtree by just moving all the leaves, and mark
all the leaves as dirty, they're still next to each other and don't need rehashing.

Also, moves have to be in order, as you may get 8->4 followed by 12->8.

Other idea: there's only one type, and whether it is move or swap is determined by
whether the source (from) is less than numLeaves.  Basically it's a swap, unless it
would be swapping with something from beyond the end of the leaves, then don't bother
copying anything outside of that.

This is ugly though as in the 8->4, 12->8 example, sure you can swap 8,4 and then
swap 12,8, where 4 ends up at 12 which is outside the forest.  But you swapped once
when you didn't have to.

... I think having everything being swaps can work an is relatively clean.
It can be of the format [a, b, h] where a and b are positions, and h is the
height, or a run of 2**h.  So [8, 0, 1] means swap 8<->0 and 9<->1.
... equivalent can just be [a, b] and a & b are *at* height h.  Same thing.

With swaps, you can take the moves and stashes and go top down and it should work.
I... *think* this means you don't have to distinguish between moves and stashes.
Also it means there's ~no memory overhead, as you never need more than 1 extra
element kept in ram (the one that's being swapped: a-> ram, b->a, ram->b )

Here's an example.  We have a full tree with 16 elements, 0...15.  0, 2 are
deleted.  removeTransform will give us (and stashes / moves don't matter), in
order:

3->0, 16->22, 25->26, 29->28

(We'll take actually the reverse order, top to bottom)
We want output:
[8, 0, 3], [12, 8, 2], [15, 2, 0]
also can be written:
[28, 29], [26, 27], [2, 15].

... write a transform function for that eh?

h:3 29->28
h:2 25->26
h:1 16->22
h:0 3->0

where'd h:1 16->22 go?  Yeah 16 ended up at 22 because of the two swaps above it...
so it turned into 22<->22 which can be omitted.

This should be useful for undo and forest remove as well.

---- more notes:

the stuff on the bottom may

*/

// topDown changes the output from removeTransform into a top-down swap list.
// give it a slice of arrows and a forest height
func topDown(ap []arrowPlus, fh uint8) []arrowPlus {
	// reverse the arrow list, now it should be top to bottom
	// TODO but you're not just making it top to bottom, but also reversing order
	// within each row.  Which is maybe not OK.
	// ... yeah it totally changes everything
	ap = upendArrowSlice(ap, fh)

	// reverseArrowSlice(ap)
	// arrows := bu
	fmt.Printf("topDown input %v\n", ap)
	// go through every entry.  Except skip the ones on the bottom row.
	for top := 0; top < len(ap); top++ {
		// mask is the xor difference between the from and to positions.
		// shift mask up by the height delta to get the xor mask to move
		// leaves around
		topMask := ap[top].from ^ ap[top].to
		topHeight := detectHeight(ap[top].from, fh)
		// modify everything underneath (not ones on the same row)
		// (but those will fail the isDescendant checks
		for sub := top + 1; sub < len(ap); sub++ {
			// redoing these a lot since they only change when height does.
			// maybe get a slice of arrows that encodes the arrow heights?
			subHeight := detectHeight(ap[sub].from, fh)
			subMask := topMask << (topHeight - subHeight)

			// swap descendents

			// swap from if under either; swap to if under same as from
			fromA, fromB := isDescendant(
				ap[sub].from, ap[top].from, ap[top].to, fh)
			toA, toB := isDescendant(
				ap[sub].to, ap[top].from, ap[top].to, fh)

			if fromA {
				// swap from
				fmt.Printf("%v causes %v -> ", ap[top], ap[sub])
				ap[sub].from ^= subMask
				fmt.Printf("%v\n", ap[sub])
			}

			if toA {
				// swap to
				fmt.Printf("%v causes %v -> ", ap[top], ap[sub])
				ap[sub].to ^= subMask
				fmt.Printf("%v\n", ap[sub])
			}

			if (toA && fromA) || (toB && fromB) {
				// swap to
				fmt.Printf("%v causes %v -> ", ap[top], ap[sub])
				ap[sub].to ^= subMask
				fmt.Printf("%v\n", ap[sub])
			}

			/*if fromA || fromB {
				// from is under either A or B; check if to is
				toA, toB := isDescendant(
					arrows[sub].to, arrows[top].from, arrows[top].to, fh)
				fmt.Printf("top %d->%d sub %d->%d\tfA %v fB %v toA %v toB %v ",
					arrows[top].from, arrows[top].to,
					arrows[sub].from, arrows[sub].to,
					fromA, fromB, toA, toB)

				if toA == fromA && toB == fromB {
					// swap to
					arrows[sub].to ^= subMask
				}
				// swap from
				arrows[sub].from ^= subMask

				fmt.Printf("became %d->%d\n", arrows[sub].from, arrows[sub].to)
			}*/
		}
	}
	// remove redundant arrows after evertyhing else is done

	/*
		for i := 0; i < len(ap); i++ {
			if ap[i].from == ap[i].to {
				if i == len(ap) {
					ap = ap[:i]
				} else {
					ap = append(ap[:i], ap[i+1:]...)
				}
				i--
			}
		}
	*/
	return ap
}

// mergerrows is ugly and does what it says but we should change
// transform itself to not need this
func mergeArrows(stash, moves []arrow) []arrowPlus {

	sp := make([]arrowPlus, len(stash))
	for i, _ := range sp {
		sp[i].from = stash[i].from
		sp[i].to = stash[i].to
		sp[i].swappy = true
	}

	mvp := make([]arrowPlus, len(moves))
	for i, _ := range mvp {
		mvp[i].from = moves[i].from
		mvp[i].to = moves[i].to
	}

	c := append(sp, mvp...)
	sortArrowPlusses(c)
	return c
}

// reverseArrowSlice does what it says.  Maybe can get rid of if we return
// the slice top-down instead of bottom-up
func reverseArrowSlice(as []arrowPlus) {
	for i, j := 0, len(as)-1; i < j; i, j = i+1, j-1 {
		as[i], as[j] = as[j], as[i]
	}
}

// upendArrowSlice moves the low arrows to the top and vice versa, while
// preserving order within individual rows
func upendArrowSlice(as []arrowPlus, h uint8) []arrowPlus {
	// currently only works one way: output is always top to bottom

	a2d := make([][]arrowPlus, h)

	for _, a := range as {
		aHeight := int(detectHeight(a.from, h))
		a2d[aHeight] = append(a2d[aHeight], a)
		fmt.Printf("%d->%d h %d\n", a.from, a.to, aHeight)
	}

	for i, row := range a2d {
		fmt.Printf(" %d %v\n", i, row)
	}

	for i, j := 0, len(a2d)-1; i < j; i, j = i+1, j-1 {
		a2d[i], a2d[j] = a2d[j], a2d[i]
	}

	z := make([]arrowPlus, 0, len(as))

	for i, row := range a2d {
		fmt.Printf("appending %d %v\n", i, row)
		z = append(z, row...)
	}

	return z
}

// topDownTransform is the removeTransform flipped to topDown by topDown()
func topDownTransform(dels []uint64, numLeaves uint64, fHeight uint8) []arrowPlus {
	stash, move := removeTransform(dels, numLeaves, fHeight)
	if len(stash) != 0 {
		fmt.Printf("*******************STASH %v\n", stash)
	}
	return topDown(mergeArrows(stash, move), fHeight)
}

// given positions p , a, and b, return 2 bools: underA, underB
// (is p is in a subtree beneath A or B)
// also returns the absolute distance an element at P's height would need to
// move to go from under A to B or vice versa.
// TODO you can do the abdist thing with XORs instead of +/- which is cooler and
// maybe get that working later.
func isDescendant(p, a, b uint64, h uint8) (bool, bool) {
	ph := detectHeight(p, h)
	abh := detectHeight(a, h)

	hdiff := abh - ph
	if hdiff == 0 || hdiff > 64 {
		return false, false
	}

	pup := upMany(p, hdiff, h)
	return pup == a, pup == b
}

// there's a clever way to do this that's faster.  But I guess it doesn't matter
// (note that this isn't it; this doesn't work)
func isDescendantClever(p, a, b uint64, h uint8) (bool, bool) {
	fmt.Printf("p %b a %b b %b\n", p, a, b)
	ph := detectHeight(p, h)
	abh := detectHeight(a, h)

	// there are really quick bitwise ways to check, if you know the heights of
	// p vs a&b.

	// we want to match h - abh bits of p and a.  Shifted by abh-ph.  I think.
	matchRange := h - abh
	shift := 64 - matchRange

	p = p << (abh - ph)
	fmt.Printf("modp %b ph %d abh %d range %d\n", p, ph, abh, matchRange)

	maskedA := ((a << (shift)) >> shift) << matchRange
	maskedB := ((b << (shift)) >> shift) << matchRange
	fmt.Printf("maskedA %b maskedB %b\n", maskedA, maskedB)
	underA := maskedA&p == 0
	// something like that...
	underB := maskedB&p == 0

	return underA, underB
}

// floorTransform calles remTrans2 and expands it to give all leaf swaps
func floorTransform(
	dels []uint64, numLeaves uint64, fHeight uint8) []arrow {
	fmt.Printf("(undo) call remTr %v nl %d fh %d\n", dels, numLeaves, fHeight)
	td := remTrans2(dels, numLeaves, fHeight)
	fmt.Printf("td output %v\n", td)

	var floor []arrow

	fmt.Printf("raw: ")
	for _, a := range td {
		fmt.Printf("%d -> %d\t", a.from, a.to)
		if a.from == a.to {
			fmt.Printf("omitting ################# %d -> %d\n", a.to, a.to)
			continue
			// TODO: why do these even exist?  get rid of them from
			// removeTransform output?
		}
		leaves := a.toLeaves(fHeight)
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

	fmt.Printf("floor: %v\n", floor)

	return floor
}

// ExpandTransform calls removeTransform with the same args, and expands its output.
// If something at height 2 moves, ExpandTransform will add moves for subnodes at
// heights 0 and 1.  The stash cutoff can now be large (with removeTransform there
// can't be more than 1 stash move per height)
func xxexpandedTransform(
	dels []uint64, numLeaves uint64, fHeight uint8) ([]arrow, []arrow, error) {
	rawStash, rawMoves := removeTransform(dels, numLeaves, fHeight)

	var expandedStash, expandedMoves []arrow
	// for each node in the stash prefix, get the whole subtree
	for _, stashPos := range rawStash {
		moves := subTreePositions(stashPos.from, stashPos.to, fHeight)
		expandedStash = append(expandedStash, moves...)
	}

	// for each node in the move section, get that whole subtree as well
	for _, movePos := range rawMoves {
		moves := subTreePositions(movePos.from, movePos.to, fHeight)
		expandedMoves = append(expandedMoves, moves...)
	}

	// populate moveMap with all expanded moves
	moveMap := make(map[uint64]uint64)
	for _, xmv := range expandedMoves {
		moveMap[xmv.from] = xmv.to
	}

	// iterate through moveMap, skipping intermediates
	// sibswap seems to cause ~most skippable sequences,
	// at least in small trees

	for firstFrom, firstTo := range moveMap {
		// is the to a from? 1 -> 2,  2 -> 3
		secondTo, ok := moveMap[firstTo]
		if ok {
			fmt.Printf("found %d -> %d -> %d, skip to %d -> %d\n",
				firstFrom, firstTo, secondTo, firstFrom, secondTo)

			parPos := up1(firstTo, fHeight)
			_, parOk := moveMap[parPos]
			fmt.Printf("move from parent %d (or above) must exist %v\n",
				parPos, parOk)

			// skip by going 1 -> 3, delete the 2 -> 3
			moveMap[firstFrom] = secondTo
			delete(moveMap, secondTo)
		}
	}

	// replace expandedMoves with the reduced moveMap.  Then sort it.
	skipMoves := make([]arrow, len(moveMap))
	i := 0
	for f, t := range moveMap {
		skipMoves[i] = arrow{from: f, to: t}
		i++
	}

	// sortMoves(expandedStash)
	// sortMoves(skipMoves)

	return expandedStash, skipMoves, nil
}
