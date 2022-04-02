package accumulator

import (
	"fmt"
	"sort"
)

// Transform outlines how the accumulator state should be modified. This function itself does
// not modify the accumulator state.
func Transform(origDels []uint64, numLeaves uint64, forestRows uint8) [][]arrow {
	dels := make([]uint64, len(origDels))
	copy(dels, origDels)

	deTwin(&dels, forestRows)

	fmt.Println("detwined del", dels)

	// Moves indicate where a leaf should move to next.
	moves := make([][]arrow, forestRows+1)

	currentRow := uint8(0)
	for _, del := range dels {
		// If the next del is not in this row, move to the next row until
		// we're at the correct row.
		for detectRow(del, forestRows) != currentRow {
			currentRow++
		}

		// If a root is being deleted, then we mark it and all the leaves below
		// it to be deleted.
		rootPresent := numLeaves&(1<<currentRow) != 0
		rootPos := rootPosition(numLeaves, currentRow, forestRows)
		if rootPresent && del == rootPos {
			moves[currentRow] = append(moves[currentRow],
				arrow{from: del, to: del})
			continue
		}

		//fmt.Printf("currentRow: %d, rootPresent: %v, rootPos: %d\n",
		//	currentRow, rootPresent, rootPos)

		sib := sibling(del)

		moves[currentRow] = append(moves[currentRow],
			arrow{from: sib, to: parent(del, forestRows)})

		//fmt.Printf("from %d, to %d\n", sib, parent(del, forestRows))

		// If 00 -> 16 and 16 -> 24, then what you're really doing is 00 -> 24.
		// The loop below tries to find any arrows that can be shortened with
		// the newly created arrow by looking at the row below.
		if currentRow != 0 {
			for i, arw := range moves[currentRow-1] {
				if arw.to == sib {
					// Change the arrow.from to the arrow.from value from
					// the row below.
					moves[currentRow][len(moves[currentRow])-1].from = arw.from

					// Delete the previous arrow from the row below.
					moves[currentRow-1] = append(
						moves[currentRow-1][:i],
						moves[currentRow-1][i+1:]...)
					break
				}
			}
		}
	}

	return moves
}

func isNextElemSibling(dels []uint64, idx int) bool {
	return dels[idx]|1 == dels[idx+1]
}

// subtreeShiftUp gives new positions for all the subtrees under. The actual leaves
// may not exist.
func subtreeShiftUp(pos uint64, forestRows uint8) {
}

// deTwin goes through the list of sorted deletions and finds the parent deletions.
// The caller MUST sort the dels before passing it into the function.
//
// Ex: If we're deleting 00 and 01 in this tree:
//
// 02
// |--\
// 00 01
//
// Then we're really deleting 02. The dels of [00, 01] would be [02].
func deTwin(dels *[]uint64, forestRows uint8) {
	for i := 0; i < len(*dels); i++ {
		// 1: Check that there's at least 2 elements in the slice left.
		// 2: Check if the right sibling of the current element matches
		//    up with the next element in the slice.
		if i+1 < len(*dels) && rightSib((*dels)[i]) == (*dels)[i+1] {
			// Grab the position of the del.
			pos := (*dels)[i]

			// Delete both of the child nodes from the slice.
			*dels = append((*dels)[:i], (*dels)[i+2:]...)

			// Calculate and Insert the parent in order.
			insertSort(dels, parent(pos, forestRows))

			// Decrement one since the next element we should
			// look at is at the same index because the slice decreased
			// in length by one.
			i--
		}
	}
}

func insertSort(dels *[]uint64, el uint64) {
	index := sort.Search(len(*dels), func(i int) bool { return (*dels)[i] > el })
	*dels = append(*dels, 0)
	copy((*dels)[index+1:], (*dels)[index:])
	(*dels)[index] = el
}

func decompressMoves(moves [][]arrow, dels []uint64) {
}

//func calcEmptyNodes(moves [][]arrow, numLeaves uint64, forestRows uint8) []uint64 {
//	emptyNodes := []uint64{}
//
//	for _, moveRow := range moves {
//		for _, move := range moveRow {
//		}
//	}
//	return nil
//}

func checkIfDescendents() {
}

func calcDirtyNodes2(moves [][]arrow, numLeaves uint64, forestRows uint8) [][]uint64 {
	dirtyNodes := make([][]uint64, len(moves))

	for currentRow := int(forestRows); currentRow >= 0; currentRow-- {
		moveRow := moves[currentRow]

		for _, move := range moveRow {
			// If to and from are the same, it means that the whole
			// subtree is gonna be deleted, resulting in no dirty nodes.
			if move.to == move.from {
				continue
			}

			// Calculate the dirty position.
			dirtyPos := parent(move.to, forestRows)

			fmt.Println("dirtyPos", dirtyPos)

			// No dirty positions if the node is moving to a root position.
			if isRootPosition(move.to, numLeaves, forestRows) {
				continue
			}

			for i := currentRow; i < len(moves); i++ {
				compMoveRow := moves[i]

				for _, compMove := range compMoveRow {
					if isAncestor(compMove.from, dirtyPos, forestRows) {

						fromRow := detectRow(compMove.from, forestRows)
						toRow := detectRow(compMove.to, forestRows)

						for currentRow := fromRow; currentRow < toRow; currentRow++ {
							fmt.Printf("compMove.from %d, compmove row %d, dirtyPos %d\n", compMove.from, currentRow, dirtyPos)

							fmt.Println("dirtypos before", dirtyPos)
							dirtyPos = calcNextPosition(dirtyPos, numLeaves, currentRow, forestRows)

							fmt.Println("dirtypos after", dirtyPos)
						}

						//delRow := detectRow(compMove.from, forestRows)
						//fmt.Printf("compMove.from %d, compmove row %d, dirtyPos %d\n", compMove.from, delRow, dirtyPos)

						//fmt.Println("dirtypos before", dirtyPos)
						//dirtyPos = calcNextPosition(dirtyPos, numLeaves, delRow, forestRows)

						//fmt.Println("dirtypos after", dirtyPos)
					} else {
						if dirtyPos == compMove.from {
							fmt.Printf("dirtyPos %d same as compMove.from. change to compMove.to %d\n",
								dirtyPos, compMove.to)
							dirtyPos = compMove.to
						}
					}
				}
			}

			// Grab the row of where the dirty position should be and
			// append to that row.
			row := detectRow(dirtyPos, forestRows)
			dirtyNodes[row] = append(dirtyNodes[row], dirtyPos)
			dirtyNodes[row] = removeDuplicateInt(dirtyNodes[row])
		}
	}

	return dirtyNodes
}

func calcNextPosition(position, numLeaves uint64, delRow, forestRows uint8) uint64 {
	fmt.Println("calcNextPosition", position)
	returnPos := getRootPosition(position, numLeaves, forestRows)

	//subTreeRows := detectSubTreeRows(position, numLeaves, forestRows)
	subTreeRows := detectRow(getRootPosition(position, numLeaves, forestRows), forestRows)
	fmt.Printf("subTreeRows %d, forestRows %d, numLeaves %d\n", subTreeRows, forestRows, numLeaves)

	positionRow := detectRow(position, forestRows)
	startRow := int(subTreeRows) - int(positionRow)

	origDelRow := delRow
	delRow = delRow - positionRow
	fmt.Printf("delrow before %d, after %d, positionRow %d\n", origDelRow, delRow, positionRow)

	if positionRow > 0 {
		beforePos := position
		mask := (1 << uint64(subTreeRows-positionRow)) - uint64(1)
		position = position & mask

		fmt.Println(position)

		fmt.Printf("pos from %d to %d with mask %d, subTreeRow %d, positionRow %d\n",
			beforePos, position, mask, subTreeRows, positionRow)
	}

	for i := int(startRow) - 1; i >= 0; i-- {
		// Skip the bit field operation for this row.
		if i == int(delRow) {
			fmt.Println("skipping row", i)
			continue
		}
		mask := uint64(1 << i)
		fmt.Println("mask", mask)

		// 1 means right
		if (position & mask) == mask {
			fmt.Println("right")
			returnPos = rightChild(returnPos, forestRows)
		} else {
			fmt.Println("left")
			returnPos = child(returnPos, forestRows)
		}
	}

	return returnPos
}

//func checkMoveUp() {
//	// Check if we're moving something that's already marked as dirty.
//	idx := slices.Index(dirtyNodes[fromRow], move.from)
//	if idx != -1 {
//		// Delete the entry on the from row.
//		dirtyNodes[fromRow] = append(dirtyNodes[fromRow][:idx],
//			dirtyNodes[fromRow][idx+1:]...)
//
//		// Add the new dirty position.
//		dirtyNodes[toRow] = append(dirtyNodes[toRow], move.to)
//	}
//}

//func didAncestorMoveUp(moves [][]arrow, position uint64, currentRow, forestRows uint8) bool {
//	for _, moveRow := range moves {
//	}
//	return false
//}

func insertDirtyPos() {
}

func isRootPosition(position, numLeaves uint64, forestRows uint8) bool {
	row := detectRow(position, forestRows)

	rootPresent := numLeaves&(1<<row) != 0
	rootPos := rootPosition(numLeaves, row, forestRows)

	fmt.Printf("pos %d, row %d, rootPresent %v, rootPos %d\n",
		position, numLeaves, rootPresent, rootPos)

	return rootPresent && rootPos == position
}

//func calcDirtyNodes(moves [][]arrow, numLeaves uint64, forestRows uint8) [][]uint64 {
//	dirtyNodes := make([][]uint64, len(moves))
//
//	for _, moveRow := range moves {
//		//for _, move := range moveRow {
//		for i := 0; i < len(moveRow); i++ {
//			move := moveRow[i]
//
//			// If to and from are the same, it means that the whole
//			// subtree is gonna be deleted, resulting in no dirty ndoes.
//			//
//			// So only calculate dirty position if they aren't the same.
//			if move.to == move.from {
//				break
//			}
//
//			fromRow := detectRow(move.from, forestRows)
//			toRow := detectRow(move.to, forestRows)
//
//			rootPresent := numLeaves&(1<<toRow) != 0
//			rootPos := rootPosition(numLeaves, uint8(toRow), forestRows)
//
//			// Check if we're moving something that's already marked as dirty.
//			idx := slices.Index(dirtyNodes[fromRow], move.from)
//			if idx != -1 {
//				// Delete the entry on the from row.
//				dirtyNodes[fromRow] = append(dirtyNodes[fromRow][:idx],
//					dirtyNodes[fromRow][idx+1:]...)
//
//				// Add the new dirty position.
//				dirtyNodes[toRow] = append(dirtyNodes[toRow], move.to)
//			}
//
//			// If we're becoming a root, there's no dirty position.
//			if rootPresent && move.to == rootPos {
//				fmt.Printf("Moving %d to root pos of %d\n",
//					move.from, move.to)
//				continue
//			}
//
//			dirtyPos := parent(move.to, forestRows)
//
//			// Grab the row of where the dirty position should be and
//			// append to that row.
//			row := detectRow(dirtyPos, forestRows)
//			dirtyNodes[row] = append(
//				dirtyNodes[row], dirtyPos)
//
//			dirtyNodes[row] = removeDuplicateInt(dirtyNodes[row])
//
//			//// If the next move.to shares the same parent, skip the
//			//// next one.
//			//if i+1 < len(moveRow) && moveRow[i+1].to^1 == move.to {
//			//	i++
//			//}
//		}
//	}
//
//	return dirtyNodes
//}

func removeDuplicateInt(uint64Slice []uint64) []uint64 {
	allKeys := make(map[uint64]bool)
	list := []uint64{}
	for _, item := range uint64Slice {
		if _, value := allKeys[item]; !value {
			allKeys[item] = true
			list = append(list, item)
		}
	}
	return list
}

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
		return []arrow{{from: rootSrc, to: rootDest, collapse: true}}
	case delRemains && !rootPresent:
		// no root but 1 del: sibling becomes root & collapses
		// in this case, mark as deleted
		rootSrc := dels[len(dels)-1] ^ 1
		return []arrow{{from: rootSrc, to: rootDest, collapse: true}}
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
