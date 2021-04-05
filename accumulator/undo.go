package accumulator

import (
	"fmt"
)

/* we need to be able to undo blocks!  for bridge nodes at least.
compact nodes can just keep old roots.
although actually it can make sense for non-bridge nodes to undo as well...
*/

// TODO in general, deal with numLeaves going to 0

// blockUndo is all the data needed to undo a block: number of adds,
// and all the hashes that got deleted and where they were from
type undoBlock struct {
	numAdds   uint32   // number of adds in the block
	positions []uint64 // position of all deletions this block
	hashes    []Hash   // hashes that were deleted
}

func (u *undoBlock) ToString() string {
	s := fmt.Sprintf("- uuuu undo block %d adds\t", u.numAdds)
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

	// prevAdds := uint64(ub.numAdds)
	// prevDels := uint64(len(ub.hashes))
	// how many leaves were there at the last block?
	// prevNumLeaves := f.numLeaves + prevDels - prevAdds
	// run the transform to figure out where things came from

	fmt.Printf("post undo forest %s\n", f.ToString())
	return nil
}

// BuildUndoData makes an undoBlock from the same data that you'd give to Modify
func (f *Forest) BuildUndoData(numadds uint64, dels []uint64) *undoBlock {
	ub := new(undoBlock)
	ub.numAdds = uint32(numadds)

	// fmt.Printf("%d del, nl %d\n", len(dels), f.numLeaves)
	ub.positions = dels // the deletion positions, in sorted order
	ub.hashes = make([]Hash, len(dels))

	// populate all the hashes from the left edge of the forest
	for i, _ := range ub.positions {
		ub.hashes[i] = f.data.read(f.numLeaves + uint64(i))
		if ub.hashes[i] == empty {
			fmt.Printf("warning, wrote empty hash for position %d\n",
				ub.positions[i])
		}
	}

	return ub
}
