package accumulator

import (
	"fmt"
	"math/bits"
	"sort"
)

// verbose is a global const to get lots of printfs for debugging
var verbose = false

// ProofPositions returns the positions that are needed to prove that the targets exist.
func ProofPositions(
	targets []uint64, numLeaves uint64, forestRows uint8) ([]uint64, []uint64) {
	// the proofPositions needed without caching.
	proofPositions := make([]uint64, 0, len(targets)*int(forestRows))
	// the positions that are computed/not included in the proof.
	// (also includes the targets)
	computedPositions := make([]uint64, 0, len(targets)*int(forestRows))
	for row := uint8(0); row < forestRows; row++ {
		computedPositions = append(computedPositions, targets...)
		if numLeaves&(1<<row) > 0 && len(targets) > 0 &&
			targets[len(targets)-1] == rootPosition(numLeaves, row, forestRows) {
			// remove roots from targets
			targets = targets[:len(targets)-1]
		}

		var nextTargets []uint64
		for len(targets) > 0 {
			switch {
			// look at the first 4 targets
			case len(targets) > 3:
				if (targets[0]|1)^2 == targets[3]|1 {
					// the first and fourth target are cousins
					// => target 2 and 3 are also targets, both parents are
					// targets of next row
					nextTargets = append(nextTargets,
						parent(targets[0], forestRows), parent(targets[3], forestRows))
					targets = targets[4:]
					break
				}
				// handle first three targets
				fallthrough

			// look at the first 3 targets
			case len(targets) > 2:
				if (targets[0]|1)^2 == targets[2]|1 {
					// the first and third target are cousins
					// => the second target is either the sibling of the first
					// OR the sibiling of the third
					// => only the sibling that is not a target is appended
					// to the proof positions
					if targets[1]|1 == targets[0]|1 {
						proofPositions = append(proofPositions, targets[2]^1)
					} else {
						proofPositions = append(proofPositions, targets[0]^1)
					}
					// both parents are targets of next row
					nextTargets = append(nextTargets,
						parent(targets[0], forestRows), parent(targets[2], forestRows))
					targets = targets[3:]
					break
				}
				// handle first two targets
				fallthrough

			// look at the first 2 targets
			case len(targets) > 1:
				if targets[0]|1 == targets[1] {
					nextTargets = append(nextTargets, parent(targets[0], forestRows))
					targets = targets[2:]
					break
				}
				if (targets[0]|1)^2 == targets[1]|1 {
					proofPositions = append(proofPositions, targets[0]^1, targets[1]^1)
					nextTargets = append(nextTargets,
						parent(targets[0], forestRows), parent(targets[1], forestRows))
					targets = targets[2:]
					break
				}
				// not related, handle first target
				fallthrough

			// look at the first target
			default:
				proofPositions = append(proofPositions, targets[0]^1)
				nextTargets = append(nextTargets, parent(targets[0], forestRows))
				targets = targets[1:]
			}
		}
		targets = nextTargets
	}

	return proofPositions, computedPositions
}

// takes a slice of dels, removes the twins (in place) and returns a slice
// of parents of twins
//
// Example with 15-leaf tree in printout.txt line 21:
// If deleting the nodes 4 through 8, the function would be called with
// nodes=[4,5,6,7,8] and row=4. (note that this example different from
// others, since it has been constructed to have both parents and dels)
// It would return (parents=[18,19], dels=[8]) because with the given
// amount of rows (row=4), only 8 is not a "twin" (it has no siblings
// to be deleted, note that 7 is in a different tree than 8).
// [18, 19] are each parents of twins, since their children are nodes
// 4-7 which are to be deleted.
func extractTwins(nodes []uint64, forestRows uint8) (parents, dels []uint64) {
	for i := 0; i < len(nodes); i++ {
		if i+1 < len(nodes) && nodes[i]|1 == nodes[i+1] {
			parents = append(parents, parent(nodes[i], forestRows))
			i++ // skip one here
		} else {
			dels = append(dels, nodes[i])
		}
	}
	return
}

// detectSubTreeHight finds the rows of the subtree a given LEAF position and
// the number of leaves (& the forest rows which is redundant)
// This thing is a tricky one.  Makes a weird serpinski fractal thing if
// you map it out in a table.
// Oh wait it's pretty simple.  Go left to right through the bits of numLeaves,
// and subtract that from position until it goes negative.
// (Does not work for nodes not at the bottom)
func detectSubTreeRows(
	position uint64, numLeaves uint64, forestRows uint8) (h uint8) {
	for h = forestRows; position >= (1<<h)&numLeaves; h-- {
		position -= (1 << h) & numLeaves
	}
	return
}

// TODO optimization if it's worth it --
// in many cases detectRow is called often and you're only looking for a
// change in row.  So we could instead have a "higher" function
// where it just looks for a different number of leading 0s.

// detectRow finds the current row of your node given the node
// position and the total forest rows.. counts preceding 1 bits.
func detectRow(position uint64, forestRows uint8) uint8 {
	marker := uint64(1 << forestRows)
	var h uint8
	for h = 0; position&marker != 0; h++ {
		marker >>= 1
	}

	return h
}

// detectOffset takes a node position and number of leaves in forest, and
// returns: which subtree a node is in, the L/R bitfield to descend to the node,
// and the height from node to its tree top (which is the bitfield length).
// we return the opposite of bits, because we always invert em...
// NOTE there is a overflow that happens with position if given a leaf not in the tree
// use inForest first before calling detectOffset or you may have an infinite loop
func detectOffset(position uint64, numLeaves uint64) (uint8, uint8, uint64) {
	// TODO replace ?
	// similarities to detectSubTreeRows() with more features
	// maybe replace detectSubTreeRows with this

	// th = tree rows
	tr := treeRows(numLeaves)
	// nh = target node row
	nr := detectRow(position, tr) // there's probably a fancier way with bits...

	var biggerTrees uint8

	// add trees until you would exceed position of node

	// This is a bit of an ugly predicate.  The goal is to detect if we've
	// gone past the node we're looking for by inspecting progressively shorter
	// trees; once we have, the loop is over.

	// The predicate breaks down into 3 main terms:
	// A: pos << nr
	// B: mask
	// C: 1<<tr & numleaves (treeSize)
	// The predicate is then if (A&B >= C)
	// A is position up-shifted by the row of the node we're targeting.
	// B is the "mask" we use in other functions; a bunch of 0s at the MSB side
	// and then a bunch of 1s on the LSB side, such that we can use bitwise AND
	// to discard high bits. Together, A&B is shifting position up by nr bits,
	// and then discarding (zeroing out) the high bits.  This is the same as in
	// childMany. C checks for whether a tree exists at the current tree
	// rows. If there is no tree at th, C is 0. If there is a tree, it will
	// return a power of 2: the base size of that tree.
	// The C term actually is used 3 times here, which is ugly; it's redefined
	// right on the next line.
	// In total, what this loop does is to take a node position, and
	// see if it's in the next largest tree.  If not, then subtract everything
	// covered by that tree from the position, and proceed to the next tree,
	// skipping trees that don't exist.

	for ; (position<<nr)&((2<<tr)-1) >= (1<<tr)&numLeaves; tr-- {
		treeSize := (1 << tr) & numLeaves
		if treeSize != 0 {
			position -= treeSize
			biggerTrees++
		}
	}

	return biggerTrees, tr - nr, ^position
}

// child gives you the left child (LSB will be 0)
func child(position uint64, forestRows uint8) uint64 {
	mask := uint64(2<<forestRows) - 1
	return (position << 1) & mask
}

// go down drop times (always left; LSBs will be 0) and return position
func childMany(position uint64, drop, forestRows uint8) uint64 {
	if drop == 0 {
		return position
	}
	if drop > forestRows {
		panic("childMany drop > forestRows")
	}
	mask := uint64(2<<forestRows) - 1
	return (position << drop) & mask
}

// Return the position of the parent of this position
func parent(position uint64, forestRows uint8) uint64 {
	return (position >> 1) | (1 << forestRows)
}

// go up rise times and return the position
func parentMany(position uint64, rise, forestRows uint8) uint64 {
	if rise == 0 {
		return position
	}
	if rise > forestRows {
		panic("parentMany rise > forestRows")
	}
	mask := uint64(2<<forestRows) - 1
	return (position>>rise | (mask << uint64(forestRows-(rise-1)))) & mask
}

// cousin returns a cousin: the child of the parent's sibling.
// you just xor with 2.  Actually there's no point in calling this function but
// it's here to document it.  If you're the left sibling it returns the left
// cousin.
func cousin(position uint64) uint64 {
	return position ^ 2
}

// TODO  inForest can probably be done better a different way.
// do we really need this at all?  only used for error detection in descendToPos

// check if a node is in a forest based on number of leaves.
// go down and right until reaching the bottom, then check if over numleaves
// (same as childmany)
// TODO fix.  says 14 is inforest with 5 leaves...
func inForest(pos, numLeaves uint64, forestRows uint8) bool {
	// quick yes:
	if pos < numLeaves {
		return true
	}
	marker := uint64(1 << forestRows)
	mask := (marker << 1) - 1
	if pos >= mask {
		return false
	}
	for pos&marker != 0 {
		pos = ((pos << 1) & mask) | 1
	}
	return pos < numLeaves
}

// treeRows returns the number of rows given n leaves.
func treeRows(n uint64) uint8 {
	// treeRows works by:
	// 1. Find the next power of 2 from the given n leaves.
	// 2. Calculate the log2 of the result from step 1.
	//
	// For example, if the given number is 9, the next power of 2 is
	// 16. This log2 of this number is how many rows there are in the
	// given tree.
	//
	// This works because while Utreexo is a collection of perfect
	// trees, the allocated number of leaves is always a power of 2.
	// For Utreexo trees that don't have leaves that are power of 2,
	// the extra space is just unallocated/filled with zeros.

	// Find the next power of 2
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	n |= n >> 32
	n++

	// log of 2 is the tree depth/height
	// if n == 0, there will be 64 traling zeros but actually no tree rows.
	// we clear the 6th bit to return 0 in that case.
	return uint8(bits.TrailingZeros64(n) & ^int(64))

}

// numRoots returns the number of 1 bits in n.
func numRoots(n uint64) uint8 {
	return uint8(bits.OnesCount64(n))
}

// rootPosition: given a number of leaves and a row, find the position of the
// root at that row.  Does not return an error if there's no root at that
// row so watch out and check first.  Checking is easy: leaves & (1<<h)
func rootPosition(leaves uint64, h, forestRows uint8) uint64 {
	mask := uint64(2<<forestRows) - 1
	before := leaves & (mask << (h + 1))
	shifted := (before >> h) | (mask << (forestRows + 1 - h))
	return shifted & mask
}

// getRootsForwards gives you the positions of the tree roots, given a number of leaves.
func getRootsForwards(leaves uint64, forestRows uint8) (roots []uint64, rows []uint8) {
	position := uint64(0)

	for row := forestRows; position < leaves; row-- {
		if (1<<row)&leaves != 0 {
			// build a tree here
			root := parentMany(position, row, forestRows)

			roots = append(roots, root)
			rows = append(rows, row)
			position += 1 << row
		}
	}

	return
}

// subTreePositions takes in a node position and forestRows and returns the
// positions of all children that need to move AND THE NODE ITSELF.  (it works
// nicer that way)
// Also it returns where they should move to, given the destination of the
// sub-tree root.
// can also be used with the "to" return discarded to just enumerate a subtree
// swap tells whether to activate the sibling swap to try to preserve order
func subTreePositions(
	subroot uint64, moveTo uint64, forestRows uint8) (as []arrow) {

	subRow := detectRow(subroot, forestRows)
	//	fmt.Printf("node %d row %d\n", subroot, subRow)
	rootDelta := int64(moveTo) - int64(subroot)
	// do this with nested loops instead of recursion ... because that's
	// more fun.
	// r is out row in the forest
	// start at the bottom and ascend
	for r := uint8(0); r <= subRow; r++ {
		// find leftmost child on this row; also calculate the
		// delta (movement) for this row
		depth := subRow - r
		leftmost := childMany(subroot, depth, forestRows)
		rowDelta := rootDelta << depth // usually negative
		for i := uint64(0); i < 1<<depth; i++ {
			// loop left to right
			f := leftmost + i
			t := uint64(int64(f) + rowDelta)
			as = append(as, arrow{from: f, to: t})
		}
	}

	return
}

// TODO: unused? useless?
// subTreeLeafRange gives the range of leaves under a node
func subTreeLeafRange(
	subroot uint64, forestRows uint8) (uint64, uint64) {

	h := detectRow(subroot, forestRows)
	left := childMany(subroot, h, forestRows)
	run := uint64(1 << h)

	return left, run
}

// to leaves takes a arrow and returns a slice of arrows that are all the
// leaf arrows below it
func (a *arrow) toLeaves(h, forestRows uint8) []arrow {
	if h == 0 {
		return []arrow{*a}
	}

	run := uint64(1 << h)
	fromStart := childMany(a.from, h, forestRows)
	toStart := childMany(a.to, h, forestRows)

	leaves := make([]arrow, run)
	for i := uint64(0); i < run; i++ {
		leaves[i] = arrow{from: fromStart + i, to: toStart + i}
	}

	return leaves
}

// it'd be cool if you just had .sort() methods on slices of builtin types...
func sortUint64s(s []uint64) {
	sort.Slice(s, func(a, b int) bool { return s[a] < s[b] })
}

func sortNodeSlice(s []node) {
	sort.Slice(s, func(a, b int) bool { return s[a].Pos < s[b].Pos })
}

// checkSortedNoDupes returns true for strictly increasing slices
func checkSortedNoDupes(s []uint64) bool {
	for i, _ := range s {
		if i == 0 {
			continue
		}
		if s[i-1] >= s[i] {
			return false
		}
	}
	return true
}

// TODO is there really no way to just... reverse any slice?  Like with
// interface or something?  it's just pointers and never touches the actual
// type...

// reverseArrowSlice does what it says.  Maybe can get rid of if we return
// the slice top-down instead of bottom-up
func reverseArrowSlice(a []arrow) {
	for i, j := 0, len(a)-1; i < j; i, j = i+1, j-1 {
		a[i], a[j] = a[j], a[i]
	}
}

// exact same code twice, couldn't you have a reverse *any* slice func...?
// but maybe that's generics or something
func reverseUint64Slice(a []uint64) {
	for i, j := 0, len(a)-1; i < j; i, j = i+1, j-1 {
		a[i], a[j] = a[j], a[i]
	}
}

func reversePolNodeSlice(a []*polNode) {
	for i, j := 0, len(a)-1; i < j; i, j = i+1, j-1 {
		a[i], a[j] = a[j], a[i]
	}
}

// mergeSortedSlices takes two slices (of uint64s; though this seems
// generalizable in that it's just < and > operators) and merges them into
// a single sorted slice, discarding duplicates.  (but not detecting or discarding
// duplicates within a single slice)
// (eg [1, 5, 8, 9], [2, 3, 4, 5, 6] -> [1, 2, 3, 4, 5, 6, 8, 9]
func mergeSortedSlices(a []uint64, b []uint64) (c []uint64) {
	maxa := len(a)
	maxb := len(b)

	// shortcuts:
	if maxa == 0 {
		return b
	}
	if maxb == 0 {
		return a
	}

	// make it (potentially) too long and truncate later
	c = make([]uint64, maxa+maxb)

	idxa, idxb := 0, 0
	for j := 0; j < len(c); j++ {
		// if we're out of a or b, just use the remainder of the other one
		if idxa >= maxa {
			// a is done, copy remainder of b
			j += copy(c[j:], b[idxb:])
			c = c[:j] // truncate empty section of c
			break
		}
		if idxb >= maxb {
			// b is done, copy remainder of a
			j += copy(c[j:], a[idxa:])
			c = c[:j] // truncate empty section of c
			break
		}

		vala, valb := a[idxa], b[idxb]
		if vala < valb { // a is less so append that
			c[j] = vala
			idxa++
		} else if vala > valb { // b is less so append that
			c[j] = valb
			idxb++
		} else { // they're equal
			c[j] = vala
			idxa++
			idxb++
		}
	}
	return
}

// dedupeSwapDirt is kind of like mergeSortedSlices.  Takes 2 sorted slices
// a, b and removes all elements of b from a and returns a.
// in this case b is arrow.to
func dedupeSwapDirt(a []uint64, b []arrow) []uint64 {
	maxa := len(a)
	maxb := len(b)
	var c []uint64
	// shortcuts:
	if maxa == 0 || maxb == 0 {
		return a
	}
	idxb := 0
	for j := 0; j < maxa; j++ {
		// skip over swap destinations less than current dirt
		for idxb < maxb && b[idxb].to < a[j] {
			idxb++
		}
		if idxb == maxb { // done
			c = append(c, a[j:]...)
			break
		}
		if a[j] != b[idxb].to {
			c = append(c, a[j])
		}
	}

	return c
}

// BinString prints out the whole thing.  Only viable for small forests
func BinString(leaves uint64) string {
	fh := treeRows(leaves)

	// tree rows should be 6 or less
	if fh > 6 {
		return "forest too big to print "
	}

	output := make([]string, (fh*2)+1)
	var pos uint8
	for h := uint8(0); h <= fh; h++ {
		rowlen := uint8(1 << (fh - h))

		for j := uint8(0); j < rowlen; j++ {
			//			if pos < uint8(leaves) {
			output[h*2] += fmt.Sprintf("%05b ", pos)
			//			} else {
			//				output[h*2] += fmt.Sprintf("       ")
			//			}

			if h > 0 {
				//				if x%2 == 0 {
				output[(h*2)-1] += "|-----"
				for q := uint8(0); q < ((1<<h)-1)/2; q++ {
					output[(h*2)-1] += "------"
				}
				output[(h*2)-1] += "\\     "
				for q := uint8(0); q < ((1<<h)-1)/2; q++ {
					output[(h*2)-1] += "      "
				}

				//				}

				for q := uint8(0); q < (1<<h)-1; q++ {
					output[h*2] += "      "
				}

			}
			pos++
		}

	}
	var s string
	for z := len(output) - 1; z >= 0; z-- {
		s += output[z] + "\n"
	}
	return s
}
