package utreexo

import "fmt"

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
	s := fmt.Sprintf("undo block %d adds\t", u.adds)
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

	// how many leaves were there at the last block?
	prevNumLeaves := f.numLeaves + uint64(len(ub.positions)) - uint64(ub.adds)

	// run the transform to figure out where things came from

	stash, moves, leaf := transformLeafUndo(ub.positions, prevNumLeaves, f.height)

	fmt.Printf("\t\t### UNDO DATA\n")
	fmt.Printf("stash %v\n", stash)
	fmt.Printf("moves %v\n", moves)
	fmt.Printf("leaf moves %v\n", leaf)

	return nil
}

// BuildUndoData makes an undoBlock from the same data that you'd had to Modify
func (f *Forest) BuildUndoData(adds []LeafTXO, dels []uint64) *undoBlock {
	ub := new(undoBlock)
	ub.adds = uint32(len(adds))

	ub.positions = dels
	ub.hashes = make([]Hash, len(dels))

	for i, pos := range ub.positions {
		ub.hashes[i] = f.forest[pos]
	}

	return ub
}
