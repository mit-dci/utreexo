package tree

import (
	"fmt"
	"os"
	"time"

	"github.com/mit-dci/utreexo/transform"
	"github.com/mit-dci/utreexo/util"
)

// For verbosity during testing
const sibSwap = false
const bridgeVerbose = false

/*
This means that in most cases there will be null nodes in the tree.
That's OK; it helps reduce renumbering nodes and makes it easier to think about
*/

// Forest is the entire accumulator of the UTXO set as either a:
// 1) slice if the forest is stored in memory.
// 2) single file if the forest is stored in disk.
// A leaf represents a UTXO with additional data for verification.
// This leaf is numbered from bottom left to right.
// Example of a forest with 4 numLeaves:
//
// row: 2      06
//             |---------\
// row: 1      04        05
//             |----\    |---\
// row: 0      00---01---02---03
//
// 04 is the concatenation and the hash of 00 and 01. 06 is the root
// This tree would have a height of 2.
type Forest struct {
	// number of leaves in the forest. Represents the amount of hashes in
	// row 0
	numLeaves uint64

	// height of the forest. NOTE: NON-INTUITIVE
	//
	// When there is only 1 tree in the forest, it is equal to the height of
	// that tree (2**h nodes). If there are multiple trees, height will
	// be 1 higher than the highest tree in the forest.
	//
	// While you could just run treeHeight(numLeaves), and pollard does just this,
	// here it incurs the cost of a reMap() when you cross a power of 2 boundary.
	//
	// Currently, the algorithm calls reMap() on the way up, but NOT on the
	// way down, so the height can sometimes be higher than it would be as
	// treeHeight(numLeaves).
	//
	// Could remove this, but likely would have a performance
	// penalty if the set dances right above / below a power of 2 leaves.
	height uint8

	// data is an interface for storing the forest leaves. This is just row
	// 0. Currently, the implementation supports ram based and a disk based
	// maybe if one is clearly better can go back to non-interface.
	data ForestData

	// positionMap maps hashes to positions
	// MiniHash represents the 12 byte chopped hash slice of a LeafTXO.
	// uint64 represents the positions. Example can be seen in the above
	// tree.
	positionMap map[util.MiniHash]uint64

	/*
	 * below are just for testing / benchmarking
	 */

	// HistoricHashes represents how many hashes this forest has computed
	HistoricHashes uint64

	// TimeRem represents how long Remove() function took
	TimeRem time.Duration

	// TimeMST represents how long the moveSubTree() function took
	TimeMST time.Duration

	// TimeInHash represents how long the hash operations (reHash) took
	TimeInHash time.Duration

	// TimeInProve represents how long the Prove operations took
	TimeInProve time.Duration

	// TimeInVerify represents how long the verify operations took
	TimeInVerify time.Duration
}

// NewForest initializes a forest.
// argument 'nil' will generate the forest in RAM
func NewForest(forestFile *os.File) *Forest {
	f := new(Forest)
	f.numLeaves = 0
	f.height = 0

	if forestFile == nil {
		// for in-ram
		f.data = new(ramForestData)
	} else {
		// for on-disk
		d := new(diskForestData)
		d.f = forestFile
		f.data = d
	}

	f.data.resize(1)
	f.positionMap = make(map[util.MiniHash]uint64)
	return f
}

// empty is needed for detection (to find errors) but I'm not sure it's needed
// for deletion. I think you can just leave garbage around, as it'll either
// get immediately overwritten, or it'll be out to the right, beyond the edge
// of the forest
var empty [32]byte

// removev4 is the fourth iteration of the remove algorithm
// removes the given slice of leaf positions
func (f *Forest) removev4(dels []uint64) error {
	// TODO: forest.removev4 and pollard.rem2 are VERY similar.
	// It seems like most of the complicated stuff is the same.
	// An interface generation could be possible.
	// In the case of remove, the only specific calls are
	// HnFromPos and swapNodes

	// Calculate the numLeaves for the forest after deletions happen
	nextNumLeaves := f.numLeaves - uint64(len(dels))

	// check that all the positions of the leaves to delete aren't
	// bigger than the numLeaves
	for _, dpos := range dels {
		if dpos > f.numLeaves {
			return fmt.Errorf(
				"Trying to delete leaf at %d, beyond max %d",
				dpos, f.numLeaves)
		}
	}
	// Dirty hashes that should be hashed again/removed
	var hashDirt, nextHashDirt []uint64
	var prevHash uint64
	var err error
	swaprows := transform.RemTrans(dels, f.numLeaves, f.height)
	// loop taken from pollard rem2.  maybe pollard and forest can both
	// satisfy the same interface..?  maybe?  that could work...
	// TODO try that ^^^^^^
	for h := uint8(0); h < f.height; h++ {
		var hdestslice []uint64
		var hashdest uint64
		hashDirt = util.DedupeSwapDirt(hashDirt, swaprows[h])

		// if there is anything left to swap or if any hash is dirty
		for len(swaprows[h]) != 0 || len(hashDirt) != 0 {
			// check if doing dirt. if not dirt, swap.
			// (maybe a little clever here...)
			if len(swaprows[h]) == 0 ||
				len(hashDirt) != 0 && hashDirt[0] > swaprows[h][0].To {
				// re-descending here which isn't great
				// fmt.Printf("hashing from dirt %d\n", hashDirt[0])
				hashdest = util.Up1(hashDirt[0], f.height)
				hashDirt = hashDirt[1:]
			} else { // swapping

				err = f.swapNodes(swaprows[h][0], h)
				if err != nil {
					return err
				}
				hashdest = util.Up1(swaprows[h][0].To, f.height)
				swaprows[h] = swaprows[h][1:]
			}
			if !util.InForest(hashdest, f.numLeaves, f.height) || hashdest == 0 {
				continue
				// TODO would be great to use nextNumLeaves... but tricky
			}
			if hashdest == prevHash { // we just did this
				// fmt.Printf("just did %d\n", prevHash)
				continue // TODO this doesn't cover eveything
			}
			hdestslice = append(hdestslice, hashdest)
			// fmt.Printf("added hp %d\n", hashdest)
			prevHash = hashdest
			if len(nextHashDirt) == 0 ||
				(nextHashDirt[len(nextHashDirt)-1] != hashdest) {
				// skip if already on end of slice. redundant?
				nextHashDirt = append(nextHashDirt, hashdest)
			}
		}
		hashDirt = nextHashDirt
		nextHashDirt = []uint64{}
		// do all the hashes at once at the end
		err := f.hashRow(hdestslice)
		if err != nil {
			return err
		}
	}
	f.numLeaves = nextNumLeaves

	return nil
}

func (f *Forest) swapNodes(s util.Arrow, height uint8) error {
	if s.From == s.To {
		// these shouldn't happen, and seems like the don't

		fmt.Printf("%s\nmove %d to %d\n", f.ToString(), s.from, s.to)
		panic("got non-moving swap")
	}
	if height == 0 {
		f.data.swapHash(s.From, s.To)
		f.positionMap[f.data.read(s.To).Mini()] = s.To
		f.positionMap[f.data.read(s.From).Mini()] = s.From
		return nil
	}
	a := util.ChildMany(s.From, height, f.height)
	b := util.ChildMany(s.To, height, f.height)
	run := uint64(1 << height)

	// happens before the actual swap, so swapping a and b
	for i := uint64(0); i < run; i++ {
		f.positionMap[f.data.read(a+i).Mini()] = b + i
		f.positionMap[f.data.read(b+i).Mini()] = a + i
	}

	// start at the bottom and go to the top
	for h := uint8(0); h <= height; h++ {
		// fmt.Printf("shr %d %d %d\n", a, b, run)
		f.data.swapHashRange(a, b, run)
		a = util.Up1(a, f.height)
		b = util.Up1(b, f.height)
		run >>= 1
	}

	return nil
}

// reHash hashes new data in the forest based on dirty positions.
// right now it seems "dirty" means the node itself moved, not that the
// parent has changed children.
// TODO: switch the meaning of "dirt" to mean parents with changed children;
// this will probably make it a lot simpler.
func (f *Forest) reHash(dirt []uint64) error {
	if f.height == 0 || len(dirt) == 0 { // nothing to hash
		return nil
	}
	tops, topheights := util.GetTopsReverse(f.numLeaves, f.height)

	dirty2d := make([][]uint64, f.height)
	h := uint8(0)
	dirtyRemaining := 0
	for _, pos := range dirt {
		if pos > f.numLeaves {
			return fmt.Errorf("Dirt %d exceeds numleaves %d", pos, f.numLeaves)
		}
		dHeight := util.DetectHeight(pos, f.height)
		// increase height if needed
		for h < dHeight {
			h++
		}
		if h > f.height {
			return fmt.Errorf("position %d at height %d but forest only %d high",
				pos, h, f.height)
		}
		// if bridgeVerbose {
		// fmt.Printf("h %d\n", h)
		// }
		dirty2d[h] = append(dirty2d[h], pos)
		dirtyRemaining++
	}

	// this is basically the same as VerifyBlockProof.  Could maybe split
	// it to a separate function to reduce redundant code..?
	// nah but pretty different because the dirtyMap has stuff that appears
	// halfway up...

	var currentRow, nextRow []uint64

	// floor by floor
	for h = uint8(0); h < f.height; h++ {
		if bridgeVerbose {
			fmt.Printf("dirty %v\ncurrentRow %v\n", dirty2d[h], currentRow)
		}

		// merge nextRow and the dirtySlice.  They're both sorted so this
		// should be quick.  Seems like a CS class kindof algo but who knows.
		// Should be O(n) anyway.

		currentRow = util.MergeSortedSlices(currentRow, dirty2d[h])
		dirtyRemaining -= len(dirty2d[h])
		if dirtyRemaining == 0 && len(currentRow) == 0 {
			// done hashing early
			break
		}

		for i, pos := range currentRow {
			// skip if next is sibling
			if i+1 < len(currentRow) && currentRow[i]|1 == currentRow[i+1] {
				continue
			}
			if len(tops) == 0 {
				return fmt.Errorf(
					"currentRow %v no tops remaining, this shouldn't happen",
					currentRow)
			}
			// also skip if this is a top
			if pos == tops[0] {
				continue
			}

			right := pos | 1
			left := right ^ 1
			parpos := util.Up1(left, f.height)

			//				fmt.Printf("bridge hash %d %04x, %d %04x -> %d\n",
			//					left, leftHash[:4], right, rightHash[:4], parpos)
			if f.data.read(left) == empty || f.data.read(right) == empty {
				f.data.write(parpos, empty)
			} else {
				par := util.Parent(f.data.read(left), f.data.read(right))
				f.HistoricHashes++
				f.data.write(parpos, par)
			}
			nextRow = append(nextRow, parpos)
		}
		if topheights[0] == h {
			tops = tops[1:]
			topheights = topheights[1:]
		}
		currentRow = nextRow
		nextRow = []uint64{}
	}

	return nil
}

// cleanup removes extraneous hashes from the forest.  Currently only the bottom
// Probably don't need this at all, if everything else is working.
func (f *Forest) cleanup(overshoot uint64) {
	for p := f.numLeaves; p < f.numLeaves+overshoot; p++ {
		delete(f.positionMap, f.data.read(p).Mini()) // clear position map
		// TODO ^^^^ that probably does nothing. or at least should...
		// f.data.write(p, empty) // clear forest
	}
}

// Add adds leaves to the forest with the given LeafTXOs
func (f *Forest) Add(adds []util.LeafTXO) {
	for _, add := range adds {
		// Add the 12 byte hash as key and current numLeaf as value
		f.positionMap[add.Mini()] = f.numLeaves

		// grab the positions of the roots (aka tops)
		tops, _ := util.GetTopsReverse(f.numLeaves, f.height)

		pos := f.numLeaves   // leaf position in the tree
		n := add.Hash        // leaf hash to add
		f.data.write(pos, n) // write n to the data with position

		for h := uint8(0); (f.numLeaves>>h)&1 == 1; h++ {
			// grab, pop, swap, hash, new

			// Grab the root/top of the current forest tree
			top := f.data.read(tops[h])

			// Hash the two leaves
			n = util.Parent(top, n)

			// Move the row up
			pos = util.Up1(pos, f.height)

			// Write the leaf
			f.data.write(pos, n)
		}
		f.numLeaves++
	}
	return
}

// Modify changes the forest, adding and deleting leaves and updating internal nodes.
// Note that this does not modify in place!  All deletes occur simultaneous with
// adds, which show up on the right.
// Also, the deletes need there to be correct proof data, so you should first call Verify().
func (f *Forest) Modify(adds []util.LeafTXO, dels []uint64) (*undoBlock, error) {
	numdels, numadds := len(dels), len(adds)
	delta := int64(numadds - numdels) // watch 32/64 bit
	if int64(f.numLeaves)+delta < 0 {
		return nil, fmt.Errorf("can't delete %d leaves, only %d exist",
			len(dels), f.numLeaves)
	}
	if !checkSortedNoDupes(dels) { // check for sorted deletion slice
		return nil, fmt.Errorf("Deletions in incorrect order or duplicated")
	}
	for _, a := range adds { // check for empty leaves
		if a.Hash == empty {
			return nil, fmt.Errorf("Can't add empty (all 0s) leaf to accumulator")
		}
	}
	// remap to expand the forest if needed
	for int64(f.numLeaves)+delta > int64(1<<f.height) {
		// fmt.Printf("current cap %d need %d\n",
		// 1<<f.height, f.numLeaves+delta)
		err := f.reMap(f.height + 1)
		if err != nil {
			return nil, err
		}
	}

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

	f.Add(adds)

	return ub, err
}

// reMap changes the height of the forest
func (f *Forest) reMap(destHeight uint8) error {

	if destHeight == f.height {
		return fmt.Errorf("can't remap %d to %d... it's the same",
			destHeight, destHeight)
	}

	if destHeight > f.height+1 || (f.height > 0 && destHeight < f.height-1) {
		return fmt.Errorf("changing by more than 1 height not programmed yet")
	}

	fmt.Printf("remap forest height %d -> %d\n", f.height, destHeight)

	// for height reduction
	if destHeight < f.height {
		// I don't think you ever need to remap down.  It really doesn't
		// matter.  Something to program someday if you feel like it for fun.
		return fmt.Errorf("height reduction not implemented")
	}
	// height increase
	f.data.resize(2 << destHeight)
	pos := uint64(1 << destHeight) // leftmost position of row 1
	reach := pos >> 1              // how much to next row up
	// start on row 1, row 0 doesn't move
	for h := uint8(1); h < destHeight; h++ {
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
	//	copy(t.fs[1<<(t.height-1):1<<t.height], make([]Hash, 1<<(t.height-1)))
	for x := uint64(1 << f.height); x < 1<<destHeight; x++ {
		// here you may actually need / want to delete?  but numleaves
		// should still ensure that you're not reading over the edge...
		f.data.write(x, empty)
	}

	f.height = destHeight
	return nil
}

// sanity checks forest sanity: does numleaves make sense, and are the tops
// populated?
func (f *Forest) sanity() error {

	if f.numLeaves > 1<<f.height {
		return fmt.Errorf("forest has %d leaves but insufficient height %d",
			f.numLeaves, f.height)
	}
	tops, _ := util.GetTopsReverse(f.numLeaves, f.height)
	for _, t := range tops {
		if f.data.read(t) == empty {
			return fmt.Errorf("Forest has %d leaves %d tops, but top @%d is empty",
				f.numLeaves, len(tops), t)
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
// miscForestFile is where numLeaves and height is stored
func RestoreForest(miscForestFile *os.File, forestFile *os.File) (*Forest, error) {

	// Initialize the forest for restore
	f := new(Forest)
	if forestFile == nil {
		// for in-ram
		f.data = new(ramForestData)
	} else {
		// for on-disk
		d := new(diskForestData)
		d.f = forestFile
		f.data = d
	}
	f.positionMap = make(map[util.MiniHash]uint64)

	// This restores the numLeaves
	var byteLeaves [8]byte
	_, err := miscForestFile.Read(byteLeaves[:])
	if err != nil {
		return nil, err
	}
	f.numLeaves = util.BtU64(byteLeaves[:])
	fmt.Println("Forest leaves:", f.numLeaves)

	// This restores the positionMap
	var i uint64
	fmt.Printf("%d iterations to do\n", f.numLeaves)
	for i = uint64(0); i < f.numLeaves; i++ {
		f.positionMap[f.data.read(i).Mini()] = i

		if i%uint64(100000) == 0 && i != uint64(0) {
			fmt.Printf("Done %d iterations\n", i)
		}
	}
	if f.positionMap == nil {
		return nil, fmt.Errorf("Generated positionMap is nil")
	}

	// This restores the height
	var byteHeight [1]byte
	_, err = miscForestFile.Read(byteHeight[:])
	if err != nil {
		return nil, err
	}
	f.height = util.BtU8(byteHeight[:])
	fmt.Println("Forest height:", f.height)
	fmt.Println("Done restoring forest")

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

// WriteForest writes the numLeaves and height to miscForestFile
func (f *Forest) WriteForest(miscForestFile *os.File) error {
	fmt.Println("numLeaves=", f.numLeaves)
	fmt.Println("f.height=", f.height)
	_, err := miscForestFile.WriteAt(append(util.U64tB(f.numLeaves), util.U8tB(f.height)...), 0)
	if err != nil {
		return err
	}
	return nil
}

// GetTops returns all the tops of the trees
func (f *Forest) GetTops() []util.Hash {

	topposs, _ := util.GetTopsReverse(f.numLeaves, f.height)
	tops := make([]util.Hash, len(topposs))

	for i := range tops {
		tops[i] = f.data.read(topposs[i])
	}

	return tops
}

// Stats :
func (f *Forest) Stats() string {

	s := fmt.Sprintf("numleaves: %d hashesever: %d posmap: %d forest: %d\n",
		f.numLeaves, f.HistoricHashes, len(f.positionMap), f.data.size())

	s += fmt.Sprintf("\thashT: %.2f remT: %.2f (of which MST %.2f) proveT: %.2f",
		f.TimeInHash.Seconds(), f.TimeRem.Seconds(), f.TimeMST.Seconds(),
		f.TimeInProve.Seconds())
	return s
}

// ToString prints out the whole thing.  Only viable for small forests
func (f *Forest) ToString() string {

	fh := f.height
	// tree height should be 6 or less
	if fh > 6 {
		return "forest too big to print "
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
				output[h*2] += fmt.Sprintf("        ")
			}
			if h > 0 {
				//				if x%2 == 0 {
				output[(h*2)-1] += "|-------"
				for q := uint8(0); q < ((1<<h)-1)/2; q++ {
					output[(h*2)-1] += "--------"
				}
				output[(h*2)-1] += "\\       "
				for q := uint8(0); q < ((1<<h)-1)/2; q++ {
					output[(h*2)-1] += "        "
				}

				//				}

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

func (f *Forest) hashRow(dirtpositions []uint64) error {

	hchan := make(chan util.HashNpos, 256) // probably don't need that big a buffer

	for _, hp := range dirtpositions {
		l := f.data.read(util.Child(hp, f.height))
		r := f.data.read(util.Child(hp, f.height) | 1)
		go util.HashOne(l, r, hp, hchan)
	}

	for remaining := len(dirtpositions); remaining > 0; remaining-- {
		hnp := <-hchan
		f.data.write(hnp.Pos, hnp.Result)
	}

	return nil
}
