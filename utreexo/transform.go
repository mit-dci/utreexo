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
Ok here's the other thing about the transform.  In many cases, you know what
to move where, but it's pointless to move it.  Actually, it's pointless
to move *any* node where any of the children have moved!  Which...
seems obvious and might speed it up / simplify it.

So the move slice can be trimmed of moves of parents.

notes in forestnotes.txt
*/

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
