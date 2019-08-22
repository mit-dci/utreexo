package utreexo

import (
	"fmt"
)

/*
The transform operations can probably be moved into a different package even.
They're some of the tricky parts of utreexo, on how to rearrange the forest nodes
when deletions occur.
*/

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
		fmt.Printf("haveDel %v rootpresent %v\n", haveDel, rootPresent)

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
			stash = append(stash, arrow{from: delPos ^ 1, to: nextTopPoss[0]})
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

*/

// topDown changes the output from removeTransform into a top-down swap list
func topDown(as []arrow) []arrow {
	// reverse the arrow list, now it should be top to bottom

	// go through every entry.  Except skip the ones on the bottom row.
	for i := 0; i < len(as); i++ {
		// modify everything underneath (not ones on the same row)
		// (but those will fail the isDescendant checks
		for j := i; j < len(as); j++ {
			// srcA, srcB := isDescendant(as[j].from, as[i].from, as[i].to)
		}

	}

	// pseudocode
	return nil
}

// given positions p , a, and b, return 2 bools: underA, underB
// (is p  is in a subtree beneath A)
func isDescendant(p, a, b uint64, h uint8) (bool, bool) {

	ph := detectHeight(p, h)
	abh := detectHeight(a, h)

	// there are really quick bitwise ways to check, if you know the heights of
	// p vs a&b.

	// we want to match h - abh bits of p and a.  Shifted by abh-ph.  I think.

	matchRange := h - abh

	p = p << (abh - ph)

	underA := (a<<(64-matchRange))>>64&p != 0
	// something like that...

	return underA, false
}

func transformLeafUndo(
	dels []uint64, numLeaves uint64, fHeight uint8) ([]arrow, []arrow, []arrow) {
	fmt.Printf("(undo) call remTr %d %d %d\n", dels, numLeaves, fHeight)
	rStashes, rMoves := removeTransform(dels, numLeaves, fHeight)

	var floor []arrow

	for _, m := range rMoves {
		if m.from < numLeaves {
			floor = append(floor, m)
		} else {
			// expand to leaves
			floor = append(floor, m.toLeaves(fHeight)...)
		}
	}
	for _, s := range rStashes {
		if s.from < numLeaves {
			floor = append(floor, s)
		} else {
			// expand to leaves
			floor = append(floor, s.toLeaves(fHeight)...)
		}
	}

	fmt.Printf("floor: %v\n", floor)

	return rStashes, rMoves, floor
}

// ExpandTransform calls removeTransform with the same args, and expands its output.
// If something at height 2 moves, ExpandTransform will add moves for subnodes at
// heights 0 and 1.  The stash cutoff can now be large (with removeTransform there
// can't be more than 1 stash move per height)
func expandedTransform(
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

	sortMoves(expandedStash)
	sortMoves(skipMoves)

	return expandedStash, skipMoves, nil
}
