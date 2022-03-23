package accumulator

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"time"

	"golang.org/x/exp/slices"
)

const (
	sibSwap       = false
	bridgeVerbose = false
)

// A FullForest is the entire accumulator of the UTXO set. This is
// what the bridge node stores.  Everything is always full.

/*
The forest is structured in the space of a tree numbered from the bottom left,
taking up the space of a perfect tree that can contain the whole forest.
This means that in most cases there will be null nodes in the tree.
That's OK; it helps reduce renumbering nodes and makes it easier to think about
addressing.  It also might work well for on-disk serialization.
There might be a better / optimal way to do this but it seems OK for now.
*/

// Forest is the entire accumulator of the UTXO set as either a:
// 1) slice if the forest is stored in memory.
// 2) single file if the forest is stored in disk.
// A leaf represents a UTXO with additional data for verification.
// This leaf is numbered from bottom left to right.
// Example of a forest with 4 numLeaves:
//
//	06
//	|------\
//	04......05
//	|---\...|---\
//	00..01..02..03
//
// 04 is the concatenation and the hash of 00 and 01. 06 is the root
// This tree would have a row of 2.
type Forest struct {
	// number of leaves in the forest (bottom row)
	numLeaves uint64

	// rows in the forest. (forest height) NON INTUITIVE!
	// When there is only 1 tree in the forest, it is equal to the rows of
	// that tree (2**h nodes).  If there are multiple trees, rows will
	// be 1 more than the tallest tree in the forest.
	// While you could just run treeRows(numLeaves), and pollard does just this,
	// here it incurs the cost of a reMap when you cross a power of 2 boundary.
	// So right now it reMaps on the way up, but NOT on the way down, so the
	// rows can sometimes be higher than it would be as treeRows(numLeaves)
	// A little weird; could remove this, but likely would have a performance
	// penalty if the set dances right above / below a power of 2 leaves.
	rows uint8

	// "data" (not the best name but) is an interface to storing the forest
	// hashes.  There's ram based and disk based for now, maybe if one
	// is clearly better can go back to non-interface.
	data ForestData

	// map from hashes to positions.
	positionMap map[MiniHash]uint64

	/*
	 * below are just for testing / benchmarking
	 */

	// historicHashes represents how many hashes this forest has computed.
	// Meant for testing / benchmarking.
	historicHashes uint64

	// timeRem represents how long Remove() function took.
	// Meant for testing / benchmarking.
	timeRem time.Duration

	// timeMST represents how long the moveSubTree() function took.
	// Meant for testing / benchmarking.
	timeMST time.Duration

	// timeInHash represents how long the hash operations (reHash) took.
	// Meant for testing / benchmarking.
	timeInHash time.Duration

	// timeInProve represents how long the Prove operations took.
	// Meant for testing / benchmarking.
	timeInProve time.Duration

	// timeInVerify represents how long the verify operations took.
	// Meant for testing / benchmarking.
	timeInVerify time.Duration
}

// ForestType defines the 4 type of forests:
// DiskForest, RamForest, CacheForest, CowForest
type ForestType int

const (
	// DiskForest  - keeps the entire forest on disk as a flat file. Is the slowest
	//               of them all. Pass an os.File as forestFile to create a DiskForest.
	DiskForest ForestType = iota
	// RamForest   - keeps the entire forest on ram as a slice. Is the fastest but
	//               takes up a lot of ram. Is compatible with DiskForest (as in you
	//               can restart as RamForest even if you created a DiskForest. Pass
	//               nil, as the forestFile to create a RamForest.
	RamForest
	// CacheForest - keeps the entire forest on disk but caches recent nodes. It's
	//               faster than disk. Is compatible with the above two forest types.
	//               Pass cached = true to create a cacheForest.
	CacheForest
	// CowForest   - A copy-on-write (really a redirect on write) forest. It strikes
	//               a balance between ram usage and speed. Not compatible with other
	//               forest types though (meaning there isn't functionality implemented
	//               to convert a CowForest to DiskForest and vise-versa). Pass a filepath
	//               and cowMaxCache(how much MB to use in ram) to create a CowForest.
	CowForest
)

// NewForest initializes a Forest and returns it. The given arguments determine
// what type of forest it will be.
func NewForest(forestType ForestType, forestFile *os.File, cowPath string, cowMaxCache int) *Forest {

	f := new(Forest)
	f.numLeaves = 0
	f.rows = 0

	switch forestType {
	case DiskForest:
		d := new(diskForestData)
		d.file = forestFile
		f.data = d
	case RamForest:
		f.data = new(ramForestData)
	case CacheForest:
		d := new(cacheForestData)
		d.file = forestFile
		d.cache = newDiskForestCache(20)
		f.data = d
	case CowForest:
		d, err := initialize(cowPath, cowMaxCache)
		if err != nil {
			panic(err)
		}
		f.data = d
	}

	f.data.resize((2 << f.rows) - 1)
	f.positionMap = make(map[MiniHash]uint64)
	return f
}

// TODO forest.removev4 and pollard.rem2 are VERY similar.  It seems like
// whether it's forest or pollard, most of the complicated stuff is the same.
// so maybe they can both satisfy an interface.  In the case of remove, the only
// specific calls are HnFromPos and swapNodes

func (f *Forest) removev4(dels []uint64) error {
	nextNumLeaves := f.numLeaves - uint64(len(dels))
	// check that all dels are there
	for _, dpos := range dels {
		if dpos > f.numLeaves {
			return fmt.Errorf(
				"Trying to delete leaf at %d, beyond max %d", dpos, f.numLeaves)
		}
	}
	var hashDirt []uint64
	swapRows := remTrans2(dels, f.numLeaves, f.rows)
	// loop taken from pollard rem2.
	// TODO Maybe pollard and forest can both satisfy the same interface..?
	for r := uint8(0); r < f.rows; r++ {
		hashDirt = updateDirt(hashDirt, swapRows[r], f.numLeaves, f.rows)
		for _, swap := range swapRows[r] {
			f.swapNodes(swap, r)
		}
		// do all the hashes at once at the end
		err := f.hashRow(hashDirt)
		if err != nil {
			return err
		}
	}
	f.numLeaves = nextNumLeaves

	return nil
}

func (f *Forest) removeSwapless(dels []uint64) error {
	// Check that all dels are there.
	for _, dpos := range dels {
		if dpos > maxPosition(f.rows) {
			return fmt.Errorf(
				"Trying to delete leaf at %d, beyond max %d", dpos, maxPosition(f.rows))
		}
	}

	moves := Transform(dels, f.numLeaves, f.rows)
	fmt.Println("moves", moves)

	// Go through all the moves from top row to the bottom row.
	for i := len(moves) - 1; i >= 0; i-- {
		moveRow := moves[i]

		for _, move := range moveRow {
			if move.from == move.to {
				f.deleteSubTree(move.from)
			} else {
				f.movePosition(move.from, move.to)
			}
		}
	}

	fmt.Println("dels", dels)
	for _, del := range dels {
		if detectRow(del, f.rows) > 0 {
			break
		}
		f.data.write(del, empty)
	}

	dirtyRows := calcDirtyNodes(moves, f.numLeaves, f.rows)

	fmt.Println("dirtyRows", dirtyRows)

	for _, dirtyRow := range dirtyRows {
		for _, dirtyPos := range dirtyRow {
			leftChild := child(dirtyPos, f.rows)
			rightChild := rightSib(leftChild)

			hash := parentHash(f.data.read(leftChild), f.data.read(rightChild))
			f.data.write(dirtyPos, hash)
		}
	}

	return nil
}

// deleteSubTree deletes the position and the entire subtree below it.
func (f *Forest) deleteSubTree(position uint64) {
	fmt.Println("deleteSubTree", position)
	fromRow := int(detectRow(position, f.rows))

	positions := []uint64{position}

	for currentRow := fromRow; currentRow >= 0; currentRow-- {
		nextPositions := []uint64{}

		for _, position := range positions {
			f.data.write(position, empty)

			// Check children and add to the list of dels.
			leftChild := child(position, f.rows)
			rightChild := rightSib(leftChild)

			if currentRow != 0 && f.data.read(leftChild) != empty {
				nextPositions = append(nextPositions, leftChild)
			}

			if currentRow != 0 && f.data.read(rightChild) != empty {
				nextPositions = append(nextPositions, rightChild)
			}
		}

		positions = nextPositions
	}
}

// movePosition moves the position and all of its children.
func (f *Forest) movePosition(from, to uint64) {
	fromRow := int(detectRow(from, f.rows))

	fromPositions := []uint64{from}
	toPositions := []uint64{to}

	for currentRow := fromRow; currentRow >= 0; currentRow-- {
		fmt.Println("from: ", fromPositions, currentRow)
		fmt.Println("to: ", toPositions)

		nextFromPositions := []uint64{}
		nextToPositions := []uint64{}

		for i, fromPosition := range fromPositions {
			toPosition := toPositions[i]

			// Grab from hash.
			fromHash := f.data.read(fromPosition)

			// Check children and add to the list of to-s and from-s if
			// they are not empty.
			leftChild := child(fromPosition, f.rows)
			rightChild := rightSib(leftChild)

			if currentRow != 0 && f.data.read(leftChild) != empty {
				nextFromPositions = append(nextFromPositions, leftChild)

				nextToPositions = append(nextToPositions,
					child(toPosition, f.rows))
			}

			if currentRow != 0 && f.data.read(rightChild) != empty {
				nextFromPositions = append(nextFromPositions, rightChild)

				nextToPositions = append(nextToPositions,
					rightSib(child(toPosition, f.rows)))
			}

			// Write the from-hash to the poition to move to.
			f.data.write(toPosition, fromHash)
		}

		// Zero out all the positions that won't have another hash replacing it.
		for i, fromPosition := range fromPositions {
			if currentRow != 0 {
				if !slices.Contains(nextToPositions, fromPosition) {
					f.data.write(fromPosition, empty)
				}
			} else {
				if detectRow(fromPosition, f.rows) == 0 {
					leftChild := child(toPositions[i], f.rows)
					rightChild := leftChild | 1

					f.deleteSubTree(leftChild)
					f.deleteSubTree(rightChild)
				} else {
					f.data.write(fromPosition, empty)
				}
			}
		}

		fromPositions = nextFromPositions
		toPositions = nextToPositions
	}
}

func (f *Forest) childrenExists(from uint64) (bool, bool) {
	leftChild := child(from, f.rows)
	rightChild := leftChild ^ 1

	left, right := false, false

	if f.data.read(leftChild) != empty {
		left = true
	}

	if f.data.read(rightChild) != empty {
		right = true
	}

	return left, right
}

func updateDirt(hashDirt []uint64, swapRow []arrow, numLeaves uint64, rows uint8) (nextHashDirt []uint64) {
	var prevHash uint64
	hashDirt = dedupeSwapDirt(hashDirt, swapRow)
	for len(swapRow) != 0 || len(hashDirt) != 0 {
		// check if doing dirt. if not dirt, swap.
		// (maybe a little clever here...)
		popSwap, hashDest := makeDestInRow(swapRow, hashDirt, rows)
		if popSwap {
			swapRow = swapRow[1:]
		} else {
			hashDirt = hashDirt[1:]
		}
		if !inForest(hashDest, numLeaves, rows) ||
			hashDest == 0 || // TODO would be great to use nextNumLeaves... but tricky
			hashDest == prevHash { // TODO this doesn't cover everything
			continue
		}
		prevHash = hashDest
		i := sort.Search(len(nextHashDirt), func(i int) bool {
			return nextHashDirt[i] >= hashDest
		})
		if i >= len(nextHashDirt) || nextHashDirt[i] != hashDest {
			// hashDest was not in the list, and i is where
			// it should be inserted
			nextHashDirt = append(nextHashDirt, 0)
			copy(nextHashDirt[i+1:], nextHashDirt[i:])
			nextHashDirt[i] = hashDest
		}
	}
	return nextHashDirt
}

func makeDestInRow(maybeArrow []arrow, hashDirt []uint64, rows uint8) (bool, uint64) {
	if len(maybeArrow) == 0 {
		// re-descending here which isn't great
		hashDest := parent(hashDirt[0], rows)
		return false, hashDest
	}

	// swapping
	hashDest := parent(maybeArrow[0].to, rows)
	return true, hashDest
}

func (f *Forest) swapNodes(s arrow, row uint8) {
	if s.from == s.to {
		// these shouldn't happen, and seems like the don't

		fmt.Printf("%s\nmove %d to %d\n", f.ToString(), s.from, s.to)
		panic("got non-moving swap")
	}
	if row == 0 {
		f.data.swapHash(s.from, s.to)
		f.positionMap[f.data.read(s.to).Mini()] = s.to
		f.positionMap[f.data.read(s.from).Mini()] = s.from
		return
	}
	a := childMany(s.from, row, f.rows)
	b := childMany(s.to, row, f.rows)
	run := uint64(1 << row)

	// happens before the actual swap, so swapping a and b
	for i := uint64(0); i < run; i++ {
		f.positionMap[f.data.read(a+i).Mini()] = b + i
		f.positionMap[f.data.read(b+i).Mini()] = a + i
	}

	// start at the bottom and go to the top
	for r := uint8(0); r <= row; r++ {
		f.data.swapHashRange(a, b, run)
		a = parent(a, f.rows)
		b = parent(b, f.rows)
		run >>= 1
	}
}

// reHash hashes new data in the forest based on dirty positions.
// right now it seems "dirty" means the node itself moved, not that the
// parent has changed children.
// TODO: switch the meaning of "dirt" to mean parents with changed children;
// this will probably make it a lot simpler.
func (f *Forest) reHash(dirt []uint64) error {
	if f.rows == 0 || len(dirt) == 0 { // nothing to hash
		return nil
	}
	positionList := NewPositionList()
	defer positionList.Free()

	rootRows := getRootsForwards(f.numLeaves, f.rows, &positionList.list)

	dirty2d := make([][]uint64, f.rows)
	r := uint8(0)
	dirtyRemaining := 0
	for _, pos := range dirt {
		if pos > f.numLeaves {
			return fmt.Errorf("Dirt %d exceeds numleaves %d", pos, f.numLeaves)
		}
		dRow := detectRow(pos, f.rows)
		// increase rows if needed
		for r < dRow {
			r++
		}
		if r > f.rows {
			return fmt.Errorf("position %d at row %d but forest only %d high",
				pos, r, f.rows)
		}
		dirty2d[r] = append(dirty2d[r], pos)
		dirtyRemaining++
	}

	// this is basically the same as VerifyBlockProof.  Could maybe split
	// it to a separate function to reduce redundant code..?
	// nah but pretty different because the dirtyMap has stuff that appears
	// halfway up...

	var currentRow, nextRow []uint64

	// floor by floor
	for r = uint8(0); r < f.rows; r++ {
		if bridgeVerbose {
			fmt.Printf("dirty %v\ncurrentRow %v\n", dirty2d[r], currentRow)
		}

		// merge nextRow and the dirtySlice.  They're both sorted so this
		// should be quick.  Seems like a CS class kind of algo but who knows.
		// Should be O(n) anyway.

		currentRow = mergeSortedSlices(currentRow, dirty2d[r])
		dirtyRemaining -= len(dirty2d[r])
		if dirtyRemaining == 0 && len(currentRow) == 0 {
			// done hashing early
			break
		}

		for i, pos := range currentRow {
			// skip if next is sibling
			if i+1 < len(currentRow) && currentRow[i]|1 == currentRow[i+1] {
				continue
			}
			if len(positionList.list) == 0 {
				return fmt.Errorf(
					"currentRow %v no roots remaining, this shouldn't happen",
					currentRow)
			}
			// also skip if this is a root
			if pos == positionList.list[len(positionList.list)-1] {
				continue
			}

			right := pos | 1
			left := right ^ 1
			parpos := parent(left, f.rows)

			if f.data.read(left) == empty || f.data.read(right) == empty {
				f.data.write(parpos, empty)
			} else {
				par := parentHash(f.data.read(left), f.data.read(right))
				f.historicHashes++
				f.data.write(parpos, par)
			}
			nextRow = append(nextRow, parpos)
		}
		if rootRows[len(rootRows)-1] == r {
			positionList.list = positionList.list[:len(rootRows)-1]
			rootRows = rootRows[:len(rootRows)-1]
		}
		currentRow = nextRow
		nextRow = nextRow[:0]
	}

	return nil
}

// cleanup removes extraneous hashes from the forest.  Currently only the bottom
// Probably don't need this at all, if everything else is working.
func (f *Forest) cleanup(overshoot uint64) {
	for p := f.numLeaves; p < f.numLeaves+overshoot; p++ {
		// TODO this probably does nothing. or at least should.
		delete(f.positionMap, f.data.read(p).Mini()) // clear position map
	}
}

// Add adds leaves to the forest.  This is the easy part.
func (f *Forest) Add(adds []Leaf) {
	f.addv2(adds)
}

// Add adds leaves to the forest.  This is the easy part.
func (f *Forest) addv2(adds []Leaf) {
	// allocate the positionList first
	positionList := NewPositionList()
	defer positionList.Free()

	for _, add := range adds {
		// reset positionList
		positionList.list = positionList.list[:0]

		f.positionMap[add.Mini()] = f.numLeaves
		getRootsForwards(f.numLeaves, f.rows, &positionList.list)
		pos := f.numLeaves
		n := add.Hash
		f.data.write(pos, n)
		add.Hash = empty

		for h := uint8(0); (f.numLeaves>>h)&1 == 1; h++ {
			rootPos := len(positionList.list) - int(h+1)
			// grab, pop, swap, hash, new
			root := f.data.read(positionList.list[rootPos]) // grab
			n = parentHash(root, n)                         // hash
			pos = parent(pos, f.rows)                       // rise
			f.data.write(pos, n)                            // write
		}
		f.numLeaves++
	}
}

func (f *Forest) addSwapless(adds []Leaf) {
	// Allocate the positionList first. This helps with garbage collection.
	positionList := NewPositionList()
	defer positionList.Free()

	for _, add := range adds {
		// Reset positionList after each loop.
		positionList.list = positionList.list[:0]

		// Add the hash to the map.
		f.positionMap[add.Mini()] = f.numLeaves

		// Grab the roots to the position list.
		getRootsForwards(f.numLeaves, f.rows, &positionList.list)
		position := f.numLeaves

		fmt.Println("all root pos: ", positionList.list)

		node := add.Hash
		f.data.write(position, node)
		add.Hash = empty

		// We can tell where the roots are by looking at the binary representation
		// of the numLeaves. Wherever there's a 1, there's a root.
		//
		// numLeaves of 8 will be '1000' in binary, so there will be one root at
		// row 3. numLeaves of 3 will be '11' in binary, so there's two roots at
		// row 0 and row 1.
		//
		// In this loop below, we're looking for these roots by checking if there's
		// a '1'. If there is, we'll hash the root being added with that root until
		// we hit a '0'.
		for h := uint8(0); (f.numLeaves>>h)&1 == 1; h++ {
			rootIdx := len(positionList.list) - int(h+1)
			root := f.data.read(positionList.list[rootIdx])

			fmt.Println("at pos: ", positionList.list[rootIdx])
			if root == empty {
				fmt.Println("root empty at pos", positionList.list[rootIdx])

				arw := arrow{from: position, to: parent(
					uint64(positionList.list[rootIdx]), f.rows)}
				f.movePosition(arw.from, arw.to)

				position = arw.to
				continue
			}

			node = parentHash(root, node)
			position = parent(position, f.rows)
			f.data.write(position, node)
			fmt.Printf("wriiting %s at pos %d\n",
				hex.EncodeToString(node[:]), position)
		}

		f.numLeaves++
	}
}

// Modify changes the forest, adding and deleting leaves and updating internal nodes.
// Note that this does not modify in place!  All deletes occur simultaneous with
// adds, which show up on the right.
// Also, the deletes need there to be correct proof data, so you should first call Verify().
func (f *Forest) Modify(adds []Leaf, delsUn []uint64) (*UndoBlock, error) {
	numdels, numadds := len(delsUn), len(adds)
	delta := int64(numadds - numdels) // watch 32/64 bit
	if int64(f.numLeaves)+delta < 0 {
		return nil, fmt.Errorf("can't delete %d leaves, only %d exist",
			len(delsUn), f.numLeaves)
	}

	// TODO for now just sort
	dels := make([]uint64, len(delsUn))
	copy(dels, delsUn)
	sortUint64s(dels)

	for _, a := range adds { // check for empty leaves
		if a.Hash == empty {
			return nil, fmt.Errorf("Can't add empty (all 0s) leaf to accumulator")
		}
	}
	// remap to expand the forest if needed
	for int64(f.numLeaves)+delta > int64(1<<f.rows) {
		// 1<<f.rows, f.numLeaves+delta)
		err := f.reMap(f.rows + 1)
		if err != nil {
			return nil, err
		}
	}

	// v3 should do the exact same thing as v2 now
	err := f.removev4(dels)
	if err != nil {
		return nil, err
	}
	f.cleanup(uint64(numdels))

	// save the leaves past the edge for undo
	// dels hasn't been mangled by remove up above, right?
	// BuildUndoData takes all the stuff swapped to the right by removev3
	// and saves it in the order it's in, which should make it go back to
	// the right place when it's swapped in reverse
	ub := f.BuildUndoData(uint64(numadds), dels)

	f.addv2(adds)

	return ub, err
}

// reMap changes the rows in the forest
func (f *Forest) reMap(destRows uint8) error {

	if destRows == f.rows {
		return fmt.Errorf("can't remap %d to %d... it's the same",
			destRows, destRows)
	}

	if destRows > f.rows+1 || (f.rows > 0 && destRows < f.rows-1) {
		return fmt.Errorf("changing by more than 1 not programmed yet")
	}

	if verbose {
		fmt.Printf("remap forest %d rows -> %d rows\n", f.rows, destRows)
	}

	// for row reduction
	if destRows < f.rows {
		return fmt.Errorf("row reduction not implemented")
	}
	// I don't think you ever need to remap down.  It really doesn't
	// matter.  Something to program someday if you feel like it for fun.
	// rows increase
	f.data.resize((2 << destRows) - 1)
	pos := uint64(1 << destRows) // leftmost position of row 1
	reach := pos >> 1            // how much to next row up
	// start on row 1, row 0 doesn't move
	for h := uint8(1); h < destRows; h++ {
		runLength := reach >> 1
		for x := uint64(0); x < runLength; x++ {
			// ok if source position is non-empty
			ok := f.data.size() > (pos>>1)+x &&
				f.data.read((pos>>1)+x) != empty
			src := f.data.read((pos >> 1) + x)
			if ok {
				f.data.write(pos+x, src)
			}
		}
		pos += reach
		reach >>= 1
	}

	// zero out (what is now the) right half of the bottom row
	//	copy(t.fs[1<<(t.rows-1):1<<t.rows], make([]Hash, 1<<(t.rows-1)))
	for x := uint64(1 << f.rows); x < 1<<destRows; x++ {
		// here you may actually need / want to delete?  but numleaves
		// should still ensure that you're not reading over the edge...
		f.data.write(x, empty)
	}

	f.rows = destRows
	return nil
}

// sanity checks forest sanity: does numleaves make sense, and are the roots
// populated?
func (f *Forest) sanity() error {

	if f.numLeaves > 1<<f.rows {
		return fmt.Errorf("forest has %d leaves but insufficient rows %d",
			f.numLeaves, f.rows)
	}

	positionList := NewPositionList()
	defer positionList.Free()

	getRootsForwards(f.numLeaves, f.rows, &positionList.list)
	for _, t := range positionList.list {
		if f.data.read(t) == empty {
			return fmt.Errorf("Forest has %d leaves %d roots, but root @%d is empty",
				f.numLeaves, len(positionList.list), t)
		}
	}

	if uint64(len(f.positionMap)) > f.numLeaves {
		return fmt.Errorf("sanity: positionMap %d leaves but forest %d leaves",
			len(f.positionMap), f.numLeaves)
	}

	return nil
}

// PosMapSanity is costly / slow: check that everything in posMap is correct
func (f *Forest) PosMapSanity() error {
	for i := uint64(0); i < f.numLeaves; i++ {
		if f.positionMap[f.data.read(i).Mini()] != i {
			return fmt.Errorf("positionMap error: map says %x @%d but @%d",
				f.data.read(i).Prefix(), f.positionMap[f.data.read(i).Mini()], i)
		}
	}
	return nil
}

// RestoreForest restores the forest on restart. Needed when resuming after exiting.
// miscForestFile is where numLeaves and rows is stored
func RestoreForest(
	miscForestFile *os.File, forestFile *os.File,
	toRAM, cached bool, cow string, cowMaxCache int) (*Forest, error) {

	// start a forest for restore
	f := new(Forest)

	// Restore the numLeaves
	err := binary.Read(miscForestFile, binary.BigEndian, &f.numLeaves)
	if err != nil {
		return nil, err
	}
	// Restore number of rows
	// TODO optimize away "rows" and only save in minimzed form
	// (this requires code to shrink the forest
	binary.Read(miscForestFile, binary.BigEndian, &f.rows)
	if err != nil {
		return nil, err
	}

	if cow != "" {
		cowData, err := loadCowForest(cow, cowMaxCache)
		if err != nil {
			return nil, err
		}

		f.data = cowData
	} else {
		// open the forest file on disk even if we're going to ram
		diskData := new(diskForestData)
		diskData.file = forestFile

		if toRAM {
			// for in-ram
			ramData := new(ramForestData)
			ramData.resize((2 << f.rows) - 1)

			// Can't read all at once!  There's a (secret? at least not well
			// documented) maxRW of 1GB.
			var bytesRead int
			for bytesRead < len(ramData.m) {
				n, err := diskData.file.Read(ramData.m[bytesRead:])
				if err != nil {
					return nil, err
				}
				bytesRead += n
			}

			f.data = ramData
		} else {
			if cached {
				// on disk, with cache
				cfd := new(cacheForestData)
				cfd.cache = newDiskForestCache(20)
				cfd.file = forestFile
				f.data = cfd
			} else {
				// on disk, no cache
				f.data = diskData
			}
			// assume no resize needed
		}
	}

	// Restore positionMap by rebuilding from all leaves
	f.positionMap = make(map[MiniHash]uint64)
	for i := uint64(0); i < f.numLeaves; i++ {
		f.positionMap[f.data.read(i).Mini()] = i
	}
	if f.positionMap == nil {
		return nil, fmt.Errorf("Generated positionMap is nil")
	}

	// for cacheForestData the `hashCount` field gets
	// set throught the size() call.
	f.data.size()

	return f, nil
}

func (f *Forest) PrintPositionMap() string {
	var s string
	for pos := uint64(0); pos < f.numLeaves; pos++ {
		l := f.data.read(pos).Mini()
		s += fmt.Sprintf("pos %d, leaf %x map to %d\n", pos, l, f.positionMap[l])
	}

	return s
}

// WriteMiscData writes the numLeaves and rows to miscForestFile
func (f *Forest) WriteMiscData(miscForestFile *os.File) error {
	err := binary.Write(miscForestFile, binary.BigEndian, f.numLeaves)
	if err != nil {
		return err
	}

	err = binary.Write(miscForestFile, binary.BigEndian, f.rows)
	if err != nil {
		return err
	}

	f.data.close()

	return nil
}

// WriteForestToDisk writes the whole forest to disk
// this only makes sense to do if the forest is in ram.  So it'll return
// an error if it's not a ramForestData
func (f *Forest) WriteForestToDisk(dumpFile *os.File, ram, cow bool) error {
	// Only the RamForest needs to be written.
	if ram {
		ramForest, ok := f.data.(*ramForestData)
		if !ok {
			return fmt.Errorf("WriteForest only possible with ram forest")
		}
		_, err := dumpFile.Seek(0, 0)
		if err != nil {
			return fmt.Errorf("WriteForest seek %s", err.Error())
		}
		_, err = dumpFile.Write(ramForest.m)
		if err != nil {
			return fmt.Errorf("WriteForest write %s", err.Error())
		}
	}

	return nil
}

// GetRoots returns all the roots of all the trees in the accumulator.
func (f *Forest) GetRoots() []Hash {
	positionList := NewPositionList()
	defer positionList.Free()

	getRootsForwards(f.numLeaves, f.rows, &positionList.list)
	roots := make([]Hash, len(positionList.list))

	for i, _ := range roots {
		roots[i] = f.data.read(positionList.list[i])
	}

	return roots
}

// Stats returns the current forest statics as a string. This includes
// number of total leaves, historic hashes, length of the position map,
// and the size of the forest
func (f *Forest) Stats() string {
	s := fmt.Sprintf("numleaves: %d hashesever: %d posmap: %d forest: %d\n",
		f.numLeaves, f.historicHashes, len(f.positionMap), f.data.size())
	s += fmt.Sprintf("\thashT: %.2f remT: %.2f (of which MST %.2f) proveT: %.2f",
		f.timeInHash.Seconds(), f.timeRem.Seconds(), f.timeMST.Seconds(),
		f.timeInProve.Seconds())

	return s
}

// ToString prints out the whole thing.  Only viable for small forests
func (f *Forest) ToString() string {

	fh := f.rows
	// tree rows should be 6 or less
	if fh > 6 {
		s := fmt.Sprintf("can't print %d leaves. roots:\n", f.numLeaves)
		roots := f.GetRoots()
		for i, r := range roots {
			s += fmt.Sprintf("\t%d %x\n", i, r.Mini())
		}
		return s
	}

	output := make([]string, (fh*2)+1)
	var pos uint8
	for h := uint8(0); h <= fh; h++ {
		rowlen := uint8(1 << (fh - h))

		for j := uint8(0); j < rowlen; j++ {
			var valstring string
			ok := f.data.size() >= uint64(pos)
			if ok {
				val := f.data.read(uint64(pos))
				if val != empty {
					valstring = fmt.Sprintf("%x", val[:2])
				}
			}
			if valstring != "" {
				output[h*2] += fmt.Sprintf("%02d:%s ", pos, valstring)
			} else {
				output[h*2] += "        "
			}
			if h > 0 {
				output[(h*2)-1] += "|-------"
				for q := uint8(0); q < ((1<<h)-1)/2; q++ {
					output[(h*2)-1] += "--------"
				}
				output[(h*2)-1] += "\\       "
				for q := uint8(0); q < ((1<<h)-1)/2; q++ {
					output[(h*2)-1] += "        "
				}

				for q := uint8(0); q < (1<<h)-1; q++ {
					output[h*2] += "        "
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

// FindLeaf finds a leave from the positionMap and returns a bool
func (f *Forest) FindLeaf(leaf Hash) bool {
	_, found := f.positionMap[leaf.Mini()]
	return found
}

// AssertEqual compares the two forests. Returns an error if the forests are not equal.
// The data meant for statics are not checked and the function will return true
// if all other fields are equal.
func (f *Forest) AssertEqual(compareForest *Forest) error {
	// Return if the number of leaves are not equal.
	if f.numLeaves != compareForest.numLeaves {
		err := fmt.Errorf("number of leaves aren't equal"+
			"forest: %d, compared forest : %d\n", f.numLeaves,
			compareForest.numLeaves)
		return err
	}

	// Preliminary check of the position map element count before looping
	// through all the elements in the map.
	if len(f.positionMap) != len(compareForest.positionMap) {
		err := fmt.Errorf("position maps sizes aren't equal"+
			"forest: %d, compared forest : %d\n", len(f.positionMap),
			len(compareForest.positionMap))
		return err
	}

	// Make sure that the two maps are equal.
	for key, val := range f.positionMap {
		compVal, ok := compareForest.positionMap[key]
		if !ok {
			err := fmt.Errorf("miniHash %s doesn't exist in the the compared forest",
				hex.EncodeToString(key[:]))
			return err
		}

		if val != compVal {
			err := fmt.Errorf("miniHash %s returned position %d for "+
				"forest but %d for the compared forest", hex.EncodeToString(key[:]),
				val, compVal)
			return err
		}
	}

	// Each forest needs its own position tracking as they may differ in the
	// actual forest rows allocated.
	var fPos, compPos uint64

	// Grab the logical rows as we're only interested in if the forests are
	// logically the same.
	logicalRows := logicalTreeRows(f.numLeaves)

	// Iterate through all the rows in the forest. The idea is that we'll
	// keep moving up, and compare all the nodes.
	for h := uint8(0); h <= logicalRows; h++ {
		// We need to re-calculate the offset as we move up each row.
		// This is because we allow the forest to allocate more space
		// than what is actually needed.
		//
		// Example: In the below tree, positions that have garbage values
		// are marked with '*'. This means that once we're at position 5,
		// we need to go to position 8 next. This is where the need to
		// re-calculate the offset comes from.
		//
		// 14*
		// |---------------\
		// 12              13*
		// |-------\       |-------\
		// 08      09      10      11*
		// |---\   |---\   |---\   |---\
		// 00  01  02  03  04  05  06* 07*
		//
		// Grab the parent of 0 for each row that we're currently on. For
		// row 1, we'll grab 08 in the above example. For row 2, we'll grab
		// 12.
		fPos = parentMany(0, h, f.rows)
		compPos = parentMany(0, h, compareForest.rows)

		// Calculate element count in the current row.
		elementCountAtRow := uint8(f.numLeaves >> h)

		// Loop through all the elements in the current row.
		for i := uint8(0); i < elementCountAtRow; i++ {
			// Read the hashes at the position from each of the forests.
			hash := f.data.read(uint64(fPos))
			compareHash := compareForest.data.read(uint64(compPos))

			// If the read hashes are not the same, return error.
			if hash != compareHash {
				err := fmt.Errorf("hashes aren't equal at forest position: %d "+
					"and compared forest position %d. "+
					"forest hash: %s compared forest hash: %s\n",
					fPos, compPos, hex.EncodeToString(hash[:]),
					hex.EncodeToString(compareHash[:]))
				return err
			}
			fPos++
			compPos++
		}
	}

	return nil
}
