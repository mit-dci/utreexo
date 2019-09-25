package utreexo

import (
	"fmt"
)

/* we need to be able to undo blocks!  for bridge nodes at least.
compact nodes can just keep old roots.
although actually it can make sense for non-bridge nodes to undo as well...
*/

// blockUndo is all the data needed to undo a block: number of adds,
// and all the hashes that got deleted and where they were from
type undoBlock struct {
	adds      uint32   // number of adds in thie block
	positions []uint64 // position of all deletions this block
	hashes    []Hash   // hashes that were overwritten or deleted
}

func (u *undoBlock) ToString() string {
	s := fmt.Sprintf("- uuuu undo block %d adds\t", u.adds)
	s += fmt.Sprintf("%d dels:\t", len(u.positions))
	if len(u.positions) != len(u.hashes) {
		s += "error"
		return s
	}
	for i, _ := range u.positions {
		s += fmt.Sprintf("%d %x,\t", u.positions[i], u.hashes[i][:4])
	}
	s += "\n"
	return s
}

// Undo : undoes one block with the undoBlock
func (f *Forest) Undo(ub undoBlock) error {

	prevAdds := uint64(ub.adds)
	// how many leaves were there at the last block?
	prevNumLeaves := f.numLeaves + uint64(len(ub.positions)) - prevAdds
	// run the transform to figure out where things came from

	// first undo the leaves added in the last block
	f.numLeaves -= prevAdds
	// clear out the hashes themselves (maybe don't need to but seems safer)
	// leaves dangling parents, but other things do that still...
	for pos := f.numLeaves; pos < f.numLeaves+prevAdds; pos++ {
		f.forest[pos] = empty
	}

	stash, moves, leaf := transformLeafUndo(ub.positions, prevNumLeaves, f.height)

	fmt.Printf("\t\t### UNDO DATA\n")
	fmt.Printf("stash %v\n", stash)
	fmt.Printf("moves %v\n", moves)
	fmt.Printf("leaf moves %d %v\n", len(leaf), leaf)
	fmt.Printf("ub hashes %d\n", len(ub.hashes))

	dirt := make([]uint64, len(leaf)*2)

	// move things backwards
	for i, a := range leaf {
		fmt.Printf("i %d a %d->%d\n", i, a.from, a.to)
		f.forest[a.from] = f.forest[a.to]
		f.forest[a.to] = ub.hashes[i]
		dirt[i*2] = up1(a.from, f.height)
		dirt[(i*2)+1] = up1(a.to, f.height)
	}

	// rehash above all tos/froms
	sortUint64s(dirt)
	fmt.Printf("rehash dirt: %v\n", dirt)
	err := f.reHash(dirt)
	if err != nil {
		return nil
	}
	f.numLeaves = prevNumLeaves
	fmt.Printf("post undo %s\n", f.ToString())
	return nil
}

// BuildUndoData makes an undoBlock from the same data that you'd had to Modify
func (f *Forest) BuildUndoData(adds []LeafTXO, dels []uint64) *undoBlock {
	ub := new(undoBlock)
	ub.adds = uint32(len(adds))

	ub.positions = dels // the deletion positions, in sorted order
	ub.hashes = make([]Hash, len(dels))

	// populate all the hashes from the forest
	for i, pos := range ub.positions {
		ub.hashes[i] = f.forest[pos]
	}

	return ub
}

func (f *Forest) reHash(dirt []uint64) error {

	tops, topheights := getTopsReverse(f.numLeaves, f.height)
	fmt.Printf("nl %d f.h %d tops %v\n", f.numLeaves, f.height, tops)
	dirty2d := make([][]uint64, f.height)
	h := uint8(0)
	dirtyRemaining := 0
	for _, pos := range dirt {
		dHeight := detectHeight(pos, f.height)
		// increase height if needed
		for h < dHeight {
			h++
		}
		// if bridgeVerbose {
		fmt.Printf("h %d\n", h)
		// }
		dirty2d[h] = append(dirty2d[h], pos)
		dirtyRemaining++
	}

	// this is basically the same as VerifyBlockProof.  Could maybe split
	// it to a separate function to reduce redundant code..?
	// nah but pretty different beacuse the dirtyMap has stuff that appears
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

		currentRow = mergeSortedSlices(currentRow, dirty2d[h])
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
			// also skip if this is a top
			if pos == tops[0] {
				continue
			}

			right := pos | 1
			left := right ^ 1
			parpos := up1(left, f.height)

			//				fmt.Printf("bridge hash %d %04x, %d %04x -> %d\n",
			//					left, leftHash[:4], right, rightHash[:4], parpos)
			if f.forest[left] == empty || f.forest[right] == empty {
				f.forest[parpos] = empty
			} else {
				par := Parent(f.forest[left], f.forest[right])
				f.HistoricHashes++
				f.forest[parpos] = par
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
