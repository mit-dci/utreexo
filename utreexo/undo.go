package utreexo

import "fmt"

/* we need to be able to undo blocks!  for bridge nodes at least.
compact nodes can just keep old roots.
although actually it can make sense for non-bridge nodes to undo as well...
*/

// blockUndo is all the data needed to undo a block: number of adds,
// and all the hashes that got deleted and where they were from
type undoBlock struct {
	adds      uint32 // how many adds; chop this much off from the right
	positions []uint64
	hashes    []Hash // hashes that were overwritten or deleted
}

// Undo : undoes one block with the undoBlock
func (f *Forest) Undo(ub undoBlock) error {

	// first cut off everything added.
	prevNumLeaves := f.numLeaves - uint64(ub.adds) + uint64(len(ub.positions))

	// run the transform to figure out where things came from
	fmt.Printf("undo transform %d rems %d prevleaves %d height\n",
		len(ub.positions), prevNumLeaves, f.height)

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
