package accumulator

import (
	"encoding/binary"
	"fmt"
	"os"
	"time"
)

const (
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
	maxLeaf   uint64 // the rightmost leaf; total adds ever
	numLeaves uint64 // number of remaining leaves in the forest

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
func NewForest(
	forestType ForestType, forestFile *os.File,
	cowPath string, cowMaxCache int) *Forest {

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

/* ------------ util functions ----------- */
// TODO remove, only here for testing
func (f *Forest) ReconstructStats() (uint64, uint8) {
	return f.numLeaves, f.rows
}

// child with forest context
func (f *Forest) child(p uint64) uint64 {
	return child(p, f.rows)
}

// parent with forest context
func (f *Forest) parent(p uint64) uint64 {
	return parent(p, f.rows)
}

// tells you if the position is a root
func (f *Forest) isRoot(p uint64, row uint8) bool {
	return f.numLeaves&(1<<row) != 0 &&
		(p == rootPosition(f.numLeaves, row, f.rows))
}

// promote moves a node up to it's parent
// returns the parent position
func (f *Forest) promote(p uint64) {
	parentPos := parent(p, f.rows)
	f.data.write(parentPos, f.data.read(p))
}

// Check if a position is the same as its parent.  Returns false if the
// parent is outside the forest.
func (f *Forest) sameAsParent(p uint64) bool {
	parPos := parent(p, f.rows)
	if !inForest(parPos, f.numLeaves, f.rows) {
		return false
	}
	selfHash := f.data.read(p)
	parHash := f.data.read(parPos)
	return selfHash == parHash
}

/*
Deletion algo
*/

// removev5 swapless
func (f *Forest) removev5(dels []uint64) error {
	// should do this later / non-blocking as it's just to free up space.
	for _, d := range dels {
		delete(f.positionMap, f.data.read(d).Mini())
	}
	dirt := make([][]uint64, f.rows)
	antidirt := make([][]uint64, f.rows)
	// consolodate deletions; only delete tops of subtrees
	condensedDels := condenseDeletions(dels, f.rows)
	// main iteration of all deletion
	for r, delRow := range condensedDels { // for each row of deletions
		for _, p := range delRow { // for each deletion originating at row r
			atRow := uint8(r)
			// first, rise until you aren't your parent
			for f.sameAsParent(p) {
				p = f.parent(p)
				atRow++
			}

			// read in sibling.  Sibling is always valid (unless you're a root)
			sib := f.data.read(p ^ 1)
			p = f.parent(p) // rise
			atRow++

			// Write sibling to position, and keep rising and writing that
			// until you're not the same as your parent
			for f.sameAsParent(p) {
				f.data.write(p, sib) // write to self
				p = f.parent(p)      // rise
				atRow++
			}
			f.data.write(p, sib) // write to self, redundant if we did prev loop

			// dirt: dirt is the parent of where you wrote to.
			// antidirt is where you wrote to. only at row 2+
			// for both dirt and anti-dirt, only write if it's not the same
			// as whats already at the end of the slice

			// Should we append antidirt?
			if atRow > 1 &&
				(len(antidirt[atRow-1]) == 0 ||
					antidirt[atRow-1][len(antidirt[atRow-1])-1] != p) {
				antidirt[atRow-1] = append(antidirt[atRow-1], p)
			}

			dirtpos := f.parent(p)
			// Should we append dirt?
			if len(dirt[atRow]) == 0 ||
				dirt[atRow][len(dirt[atRow])-1] != dirtpos {
				dirt[atRow] = append(dirt[atRow], dirtpos)
			}
		}
	}
	fmt.Printf("fr %d dirt: %v\nantidirt: %v\n", f.rows, dirt, antidirt)
	annihilate(dirt, antidirt)
	extend(dirt, getRootPositions(f.numLeaves, f.rows), f.rows)
	fmt.Printf("dirt: %v\n", dirt)
	err := f.cleanHash(dirt)
	f.numLeaves -= uint64(len(dels))
	return err
}

// zero out the antidirt from the dirt
func annihilate(d, x [][]uint64) {
	if len(d) != len(x) {
		return
	}
	for r, _ := range d {
		zeroMatchingSortedSlices(d[r], x[r])
	}
}

// extend dirt up to roots
func extend(dirt [][]uint64, rootPositions []uint64, forestRows uint8) {
	// we assume: dirt & rootpositions are same len, which is same as f.rows,
	// and a 0 value never happens in dirt

	for r := uint8(0); r < uint8(len(dirt)-1); r++ {
		var addDirt []uint64
		fmt.Printf("rootpos[%d] %d\n", r, rootPositions[r])
		for x, _ := range dirt[r] {
			if dirt[r][x] != rootPositions[r] && // not a root and
				// first, or even, or not 1 more than previous dirt
				(x == 0 || dirt[r][x]%2 == 0 || dirt[r][x] != dirt[r][x-1]+1) {
				addDirt = append(addDirt, parent(dirt[r][x], forestRows))
			}
		}
		dirt[r+1] = mergeSortedSlices(dirt[r+1], addDirt)
	}
}

// Given a list of dirty positions (positions where children have changed)
// hash & write new nodes up to the roots
func (f *Forest) cleanHash(dirt [][]uint64) error {
	n := uint64(15)
	p := uint64(4)
	fmt.Printf("nl %d tr %d parent(%d) %d\n",
		n, treeRows(n), p, parent(p, treeRows(n)))
	/*	if f.rows == 0 {
			return nil // nothing to do
		}
		if len(dirt[0]) != 0 {
			return fmt.Errorf("bottom row of dirt is not empty")
		}

		var higherDirt []uint64
		// start at row 1 and get to the top.  Row 0 is never dirty.
		for row := uint8(1); row < f.rows; row++ {
			dirt[row] = mergeSortedSlices(dirt[row], higherDirt)
			higherDirt = []uint64{}
			for _, pos := range dirt[row] {
				// TODO or maybe make a hash parent function that is part of f.data
				leftChild := f.data.read(f.child(pos)) // TODO combine with readrun
				rightChild := f.data.read(f.child(pos) | 1)
				par := parentHash(leftChild, rightChild)
				f.historicHashes++
				f.data.write(pos, par)

				if !f.isRoot(pos, row) {
					higherDirt = append(higherDirt, f.parent(pos))
				}
			}
		}
	*/
	return nil
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

	positionList := getRootPositions(f.numLeaves, f.rows)

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
			if len(positionList) == 0 {
				return fmt.Errorf(
					"currentRow %v no roots remaining, this shouldn't happen",
					currentRow)
			}
			// also skip if this is a root
			if pos == positionList[len(positionList)-1] {
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
		// if rootRows[len(rootRows)-1] == r {
		// positionList = positionList[:len(rootRows)-1]
		// rootRows = rootRows[:len(rootRows)-1]
		// }
		currentRow = nextRow
		nextRow = nextRow[:0]
	}

	return nil
}

// Add adds leaves to the forest.  This is the easy part.
func (f *Forest) Add(adds []Leaf) {
	f.addv2(adds)
	// TODO we can add faster by not doing it 1 at a time.
	// Computing getRootPositions each time is slower, and
	// could write contiguous runs per row instead of jumping around.
}

func (f *Forest) addv2(adds []Leaf) {
	for _, add := range adds {
		f.positionMap[add.Mini()] = f.numLeaves
		positionList := getRootPositions(f.numLeaves, f.rows)

		pos := f.numLeaves
		n := add.Hash
		f.data.write(pos, n)
		add.Hash = empty

		for h := uint8(0); (1<<h)&f.numLeaves != 0; h++ {
			rootPos := len(positionList) - int(h+1)
			// grab, pop, swap, hash, new
			root := f.data.read(positionList[rootPos]) // grab
			n = parentHash(root, n)                    // hash
			f.historicHashes++
			pos = parent(pos, f.rows) // rise
			f.data.write(pos, n)      // write
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

	err := f.removev5(dels)
	if err != nil {
		return nil, err
	}

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

// getRoots returns all the roots of the trees
func (f *Forest) getRoots() []Hash {
	positionList := getRootPositions(f.numLeaves, f.rows)
	roots := make([]Hash, len(positionList))

	for i, _ := range roots {
		roots[i] = f.data.read(positionList[i])
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
		roots := f.getRoots()
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
