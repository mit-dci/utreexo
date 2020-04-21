package util

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
)

// "verbose" is a global const To get lots of printfs for debugging
var Verbose = false

// DedupeHashSlices is for removing txos that get created & spent in the same block
// as adds are TTLHashes, takes those in for slice a
func DedupeHashSlices(as *[]LeafTXO, bs *[]Hash) {
	// need To preserve order, so have To do this twice...
	// build a map and b map
	ma := make(map[Hash]bool)
	for _, a := range *as {
		ma[a.Hash] = true
	}
	mb := make(map[Hash]bool)
	for _, b := range *bs {
		mb[b] = true
	}
	var anew []LeafTXO
	var bnew []Hash

	for _, a := range *as {
		_, there := mb[a.Hash]
		if !there {
			anew = append(anew, a)
		}
	}
	for _, b := range *bs {
		_, there := ma[b]
		if !there {
			bnew = append(bnew, b)
		}
	}
	*as = anew
	*bs = bnew
}

// PopCount returns the number of 1 bits in a uint64
func PopCount(i uint64) uint8 {
	var count uint8
	for i != 0 {
		if i&1 == 1 {
			count++
		}
		i >>= 1
	}
	return count
}

// ExtractTwins takes a slice of ints and extracts the adjacent ints
// which differ only in the LSB.  It then returns two slices: one of the
// *even* twins (no odds), and one of the ints with no siblings
func ExtractTwins(nodes []uint64) (twins, onlychildren []uint64) {
	// "twins" are siblings where both are deleted (I guess)

	// run through the slice of deletions, and 'dedupe' by extracting siblings
	// (if both siblings are being deleted, nothing needs To move on that row)
	for i := 0; i < len(nodes); i++ {
		if i+1 < len(nodes) && nodes[i]|1 == nodes[i+1] {
			twins = append(twins, nodes[i])
			i++ // skip one here
		} else {
			onlychildren = append(onlychildren, nodes[i])
		}
	}
	return
}

// ExTwin2 takes a slice of dels, removes the twins (in place) and returns:
// 1) a slice of parents of the twins
// 2) dels that didn't have twins
func ExTwin2(nodes []uint64, height uint8) (parents, dels []uint64) {
	for i := 0; i < len(nodes); i++ {
		// Twin if:
		// 1) i isn't the last node
		// 2) next node in the slice is +1 of the current node
		// Rule (2) works because the left child is always even
		if i+1 < len(nodes) && nodes[i]|1 == nodes[i+1] {
			parents = append(parents, Up1(nodes[i], height))
			i++ // skip one here as we Took the twin off
		} else {
			// If there's no twin, just return the del back
			dels = append(dels, nodes[i])
		}
	}
	return
}

// tree height 0 means there's 1 lead.  Tree height 1 means 2 leaves.
// so it's 1<<height leaves.  ... pretty sure about this

// DetectSubTreeHight finds the height of the subtree a given LEAF position and
// the number of leaves (& the forest height which is redundant)
// This thing is a tricky one.  Makes a weird serpinski fractal thing if
// you map it out in a table.
// Oh wait it's pretty simple.  Go left To right through the bits of numLeaves,
// and subtract that From position until it goes negative.
// (Does not work for nodes not at the botTom)
func DetectSubTreeHeight(
	position uint64, numLeaves uint64, forestHeight uint8) (h uint8) {
	for h = forestHeight; position >= (1<<h)&numLeaves; h-- {
		position -= (1 << h) & numLeaves
	}
	return
}

// TODO optimization if it's worth it --
// in many cases DetectHeight is called often and you're only looking for a
// change in height.  So we could instead have a "higher" function
// where it just looks for a different number of leading 0s.
// Actually I will write that function here

// higher returns how much higher position is than h.  if position is at height
// h, it returns 0.  if position is lower than h it's undefined (probably 0)
// untested
func Higher(position uint64, h, forestHeight uint8) uint8 {
	mask := uint64(2<<forestHeight) - 1
	mask &= mask << h // puts 0s on the right
	if position < mask {
		return 0
	}
	marker := uint64(1 << forestHeight)
	for ; position&marker != 0; h++ {
		marker >>= 1
	}
	return h
}

// DetectHeight finds the current height of your node given the node
// position and the Total forest height.. counts preceding 1 bits.
func DetectHeight(position uint64, forestHeight uint8) uint8 {
	marker := uint64(1 << forestHeight)
	var h uint8
	for h = 0; position&marker != 0; h++ {
		marker >>= 1
	}
	return h
}

// DetectOffset takes a node position and number of leaves in forest, and
// returns: which subtree a node is in, the L/R bitfield To descend To the node,
// and the height From node To its tree Top (which is the bitfield length).
// we return the opposite of bits, because we always invert em...
func DetectOffset(position uint64, numLeaves uint64) (uint8, uint8, uint64) {
	// TODO replace ?
	// similarities To detectSubTreeHeight() with more features
	// maybe replace detectSubTreeHeight with this

	// th = tree height
	th := TreeHeight(numLeaves)
	// nh = target node height
	nh := DetectHeight(position, th) // there's probably a fancier way with bits...

	var biggerTrees uint8

	// add trees until you would exceed position of node

	// This is a bit of an ugly predicate.  The goal is To detect if we've
	// gone past the node we're looking for by inspecting progressively shorter
	// trees; once we have, the loop is over.

	// The predicate breaks down inTo 3 main terms:
	// A: pos << nh
	// B: mask
	// C: 1<<th & numleaves (treeSize)
	// The predicate is then if (A&B >= C)
	// A is position up-shifted by the height of the node we're targeting.
	// B is the "mask" we use in other functions; a bunch of 0s at the MSB side
	// and then a bunch of 1s on the LSB side, such that we can use bitwise AND
	// To discard high bits.  Together, A&B is shifting position up by nh bits,
	// and then discarding (zeroing out) the high bits.  This is the same as in
	// ChildMany.  C checks for whether a tree exists at the current tree
	// height.  If there is no tree at th, C is 0.  If there is a tree, it will
	// return a power of 2: the base size of that tree.
	// The C term actually is used 3 times here, which is ugly; it's redefined
	// right on the next line.
	// In Total, what this loop does is To take a node position, and
	// see if it's in the next largest tree.  If not, then subtract everything
	// covered by that tree From the position, and proceed To the next tree,
	// skipping trees that don't exist.

	for ; (position<<nh)&((2<<th)-1) >= (1<<th)&numLeaves; th-- {
		treeSize := (1 << th) & numLeaves
		if treeSize != 0 {
			position -= treeSize
			biggerTrees++
		}
	}
	return biggerTrees, th - nh, ^position
}

// Child gives you the left child (LSB will be 0)
func Child(position uint64, forestHeight uint8) uint64 {
	mask := uint64(2<<forestHeight) - 1
	return (position << 1) & mask
}

// ChildMany go down drop times (always left; LSBs will be 0) and return position
func ChildMany(position uint64, drop, forestHeight uint8) uint64 {
	mask := uint64(2<<forestHeight) - 1
	return (position << drop) & mask
}

// Return the position of the parent of this position
func Up1(position uint64, forestHeight uint8) uint64 {
	return (position >> 1) | (1 << forestHeight)
}

// go up rise times and return the position
func UpMany(position uint64, rise, forestHeight uint8) uint64 {
	mask := uint64(2<<forestHeight) - 1
	return (position>>rise | (mask << uint64(forestHeight-(rise-1)))) & mask
}

// cousin returns a cousin: the child of the parent's sibling.
// you just xor with 2. Actually there's no point in calling this function but
// it's here To document it.  If you're the left sibling it returns the left
// cousin.
func Cousin(position uint64) uint64 {
	return position ^ 2
}

// Sibling returns the sibling of the current leaf
func Sibling(position uint64) uint64 {
	return position ^ 1
}

// TODO  inForest can probably be done better a different way.
// do we really need this at all?  only used for error detection in descendToPos

// check if a node is in a forest based on number of leaves.
// go down and right until reaching the botTom, then check if over numleaves
// (same as childmany)
// TODO fix.  says 14 is inforest with 5 leaves...
func InForest(pos, numLeaves uint64, forestHeight uint8) bool {
	// quick yes:
	if pos < numLeaves {
		return true
	}
	marker := uint64(1 << forestHeight)
	mask := (marker << 1) - 1
	if pos >= mask {
		return false
	}
	for pos&marker != 0 {
		pos = ((pos << 1) & mask) | 1
	}
	return pos < numLeaves
}

// TreeHeight, given n leaves, returns what the tree height is
// It does this by iterating and shifting left until height is greater than
// leaves
func TreeHeight(leaves uint64) (height uint8) {
	for ; (1 << height) < leaves; height++ {
	}
	return
}

// TopPos, given a number of leaves and a height, finds the position of the
// root at that height. Does not return an error if there's no root at that
// height so watch out and check first with CheckForRoot()
func TopPos(leaves uint64, h, forestHeight uint8) uint64 {
	// Turn on all bits
	mask := uint64(2<<forestHeight) - 1

	//
	before := leaves & (mask << (h + 1))
	shifted := (before >> h) | (mask << (forestHeight - (h - 1)))
	return shifted & mask
}

// checkForRoot returns if the current row(aka height) of the forest has a root.
// Example:
//
// height: 2      06
//                |---------\
// height: 1      04        05
//                |----\    |---\
// height: 0      00---01---02---03
//
// Here, only height 2 has a root. Height 1 and height 0 doesn't
func CheckForRoot(numLeaves uint64, height uint8) bool {
	// Utreexo trees always From perfect binary trees with
	// numLeaves = 2**forestHeight
	// This means any tree root in the forest will be easily checkable
	// with the below code. As decimal 4 leaves is binary 100. Only
	// the 2**2 bit is on which is where the root is.
	return numLeaves&(1<<height) != 0
}

// getTops gives you the positions of the tree Tops, given a number of leaves.
// LOWEST first (right To left) (blarg change this)
func GetTopsReverse(leaves uint64, forestHeight uint8) (Tops []uint64, heights []uint8) {
	position := uint64(0)

	// go left To right.  But append in reverse so that the Tops are low To high
	// run though all bit positions.  if there's a 1, build a tree aTop
	// the current position, and move To the right.
	for height := forestHeight; position < leaves; height-- {
		if (1<<height)&leaves != 0 {
			// build a tree here
			Top := UpMany(position, height, forestHeight)
			Tops = append([]uint64{Top}, Tops...)
			heights = append([]uint8{height}, heights...)
			position += 1 << height
		}
	}
	return
}

// subTreePositions takes in a node position and forestHeight and returns the
// positions of all children that need To move AND THE NODE ITSELF.  (it works nicer that way)
// Also it returns where they should move To, given the destination of the
// sub-tree root.
// can also be used with the "To" return discarded To just enumerate a subtree
// swap tells whether To activate the sibling swap To try To preserve order
func SubTreePositions(
	subroot uint64, moveTo uint64, forestHeight uint8) (as []Arrow) {

	subHeight := DetectHeight(subroot, forestHeight)
	//	fmt.Printf("node %d height %d\n", subroot, subHeight)
	rootDelta := int64(moveTo) - int64(subroot)
	// do this with nested loops instead of recursion ... because that's
	// more fun.
	// h is out height in the forest
	// start at the botTom and ascend
	for height := uint8(0); height <= subHeight; height++ {
		// find leftmost child at this height; also calculate the
		// delta (movement) for this row
		depth := subHeight - height
		leftmost := ChildMany(subroot, depth, forestHeight)
		rowDelta := rootDelta << depth // usually negative
		for i := uint64(0); i < 1<<depth; i++ {
			// loop left To right
			f := leftmost + i
			t := uint64(int64(f) + rowDelta)
			as = append(as, Arrow{From: f, To: t})
		}
	}

	return
}

// TODO: unused? useless?
// subTreeLeafRange gives the range of leaves under a node
func SubTreeLeafRange(
	subroot uint64, forestHeight uint8) (uint64, uint64) {

	h := DetectHeight(subroot, forestHeight)
	left := ChildMany(subroot, h, forestHeight)
	run := uint64(1 << h)

	return left, run
}

// To leaves takes a Arrow and returns a slice of Arrows that are all the
// leaf Arrows below it
func (a *Arrow) ToLeaves(h, forestHeight uint8) []Arrow {
	if h == 0 {
		return []Arrow{*a}
	}

	run := uint64(1 << h)
	FromStart := ChildMany(a.From, h, forestHeight)
	ToStart := ChildMany(a.To, h, forestHeight)

	leaves := make([]Arrow, run)
	for i := uint64(0); i < run; i++ {
		leaves[i] = Arrow{From: FromStart + i, To: ToStart + i}
	}

	return leaves
}

// it'd be cool if you just had .sort() methods on slices of builtin types...
func SortUint64s(s []uint64) {
	sort.Slice(s, func(a, b int) bool { return s[a] < s[b] })
}

func SortNodeSlice(s []Node) {
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
// the slice Top-down instead of botTom-up
func ReverseArrowSlice(a []Arrow) {
	for i, j := 0, len(a)-1; i < j; i, j = i+1, j-1 {
		a[i], a[j] = a[j], a[i]
	}
}

// exact same code twice, couldn't you have a reverse *any* slice func...?
// but maybe that's generics or something
func ReverseUint64Slice(a []uint64) {
	for i, j := 0, len(a)-1; i < j; i, j = i+1, j-1 {
		a[i], a[j] = a[j], a[i]
	}
}

// TopUp takes a slice of Arrows (in order) and returns an expanded slice of
// Arrows that contains all the parents of the given slice up To roots
func TopUp(rows [][]uint64, fh uint8) {
	// kindof inefficient since we actually start at row 1 and rows[0] is
	// always empty when we call this... but MergeSortedSlices has the
	// shortcut so shouldn't matter
	nextrow := []uint64{}
	for h := uint8(0); h <= fh; h++ { // go through each row
		fmt.Printf("h %d merge %v and %v\n", h, rows[h], nextrow)
		rows[h] = MergeSortedSlices(rows[h], nextrow)
		nextrow = []uint64{} // clear nextrow
		for i := 0; i < len(rows[h]); i++ {
			nextrow = append(nextrow, Up1(rows[h][i], fh))
			// skip the next one if it's a sibling
			if i+1 < len(rows[h]) && rows[h][i]|1 == rows[h][i+1] {
				i++
			}
		}
	}
}

// sortArrows sorts them by From
// func sortArrows(s []Arrow) {
// 	sort.Slice(s, func(a, b int) bool { return s[a].From < s[b].From })
// }

// MergeSortedSlices takes two slices (of uint64s; though this seems
// genericizable in that it's just < and > operaTors) and merges them inTo
// a single sorted slice, discarding duplicates.  (but not detecting or discarding
// duplicates within a single slice)
// (eg [1, 5, 8, 9], [2, 3, 4, 5, 6] -> [1, 2, 3, 4, 5, 6, 8, 9]
func MergeSortedSlices(a []uint64, b []uint64) (c []uint64) {
	maxa := len(a)
	maxb := len(b)

	// shortcuts:
	if maxa == 0 {
		return b
	}
	if maxb == 0 {
		return a
	}

	// make it (potentially) Too long and truncate later
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

// kindof like mergeSortedSlices.  Takes 2 sorted slices a, b and removes
// all elements of b From a and returns a.
// in this case b is Arrow.To
func DedupeSwapDirt(a []uint64, b []Arrow) []uint64 {
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
		for idxb < maxb && a[j] < b[idxb].To {
			idxb++
		}
		if idxb == maxb { // done
			c = append(c, a[j:]...)
			break
		}
		if a[j] != b[idxb].To {
			c = append(c, a[j])
		}
	}

	return c
}

// BinString prints out the whole thing.  Only viable for small forests
func BinString(leaves uint64) string {
	fh := TreeHeight(leaves)

	// tree height should be 6 or less
	if fh > 6 {
		return "forest Too big To print "
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

// BtU32 : 4 byte slice To uint32.  Returns ffffffff if something doesn't work.
func BtU32(b []byte) uint32 {
	if len(b) != 4 {
		fmt.Printf("Got %x To BtU32 (%d bytes)\n", b, len(b))
		return 0xffffffff
	}
	var i uint32
	buf := bytes.NewBuffer(b)
	binary.Read(buf, binary.BigEndian, &i)
	return i
}

// U32tB : uint32 To 4 bytes.  Always works.
func U32tB(i uint32) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, i)
	return buf.Bytes()
}

// BtU64 : 8 bytes To uint64.  returns ffff. if it doesn't work.
func BtU64(b []byte) uint64 {
	if len(b) != 8 {
		fmt.Printf("Got %x To BtU64 (%d bytes)\n", b, len(b))
		return 0xffffffffffffffff
	}
	var i uint64
	buf := bytes.NewBuffer(b)
	binary.Read(buf, binary.BigEndian, &i)
	return i
}

// U64tB : uint64 To 8 bytes.  Always works.
func U64tB(i uint64) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, i)
	return buf.Bytes()
}

// BtU8 : 1 byte To uint8.  returns ffff. if it doesn't work.
func BtU8(b []byte) uint8 {
	if len(b) != 1 {
		fmt.Printf("Got %x To BtU8 (%d bytes)\n", b, len(b))
		return 0xff
	}
	var i uint8
	buf := bytes.NewBuffer(b)
	binary.Read(buf, binary.BigEndian, &i)
	return i
}

// U8tB : uint8 To a byte.  Always works.
func U8tB(i uint8) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, i)
	return buf.Bytes()
}
