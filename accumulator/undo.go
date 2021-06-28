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

// ToString returns a string
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

// Undo reverts a Modify() with the given undoBlock.
func (f *Forest) Undo(ub undoBlock) error {
	prevAdds := uint64(ub.numAdds)
	prevDels := uint64(len(ub.hashes))
	// how many leaves were there at the last block?
	prevNumLeaves := f.numLeaves + prevDels - prevAdds
	// run the transform to figure out where things came from
	leafMoves := floorTransform(ub.positions, prevNumLeaves, f.rows)
	reverseArrowSlice(leafMoves)
	// first undo the leaves added in the last block
	f.numLeaves -= prevAdds

	// remove everything between prevNumLeaves and numLeaves from positionMap
	for p := f.numLeaves; p < f.numLeaves+prevAdds; p++ {
		delete(f.positionMap, f.data.read(p).Mini())
	}

	// also add everything past numleaves and prevnumleaves to dirt
	// which might already be there, inefficient!
	// TODO fix this dirt thing
	dirt := make([]uint64, len(leafMoves)*2)

	// place hashes starting at old post-remove numLeaves.  they're off the
	// forest bounds to the right; they will be shuffled in to the left.
	for i, h := range ub.hashes {
		if h == empty {
			return fmt.Errorf("hash %d in undoblock is empty", i)
		}
		f.data.write(f.numLeaves+uint64(i), h)
		dirt = append(dirt, f.numLeaves+uint64(i))
	}

	// go through swaps in reverse order
	for i, a := range leafMoves {
		f.data.swapHash(a.from, a.to)
		dirt[2*i] = a.to       // this is wrong, it way over hashes
		dirt[(2*i)+1] = a.from // also should be parents
	}

	// update positionMap.  The stuff we do want has been moved in to the forest,
	// the stuff we don't want has been moved to the right past the edge
	for p := f.numLeaves; p < prevNumLeaves; p++ {
		f.positionMap[f.data.read(p).Mini()] = p
	}
	for _, p := range ub.positions {
		f.positionMap[f.data.read(p).Mini()] = p
	}
	for _, d := range dirt {
		// everything that moved needs to have its position updated in the map
		// TODO does it..?
		m := f.data.read(d).Mini()
		oldpos := f.positionMap[m]
		if oldpos != d {
			delete(f.positionMap, m)
			f.positionMap[m] = d
		}
	}

	// rehash above all tos/froms
	f.numLeaves = prevNumLeaves // change numLeaves before rehashing
	sortUint64s(dirt)
	err := f.reHash(dirt)
	if err != nil {
		return err
	}

	return nil
}

// BuildUndoData makes an undoBlock from the same data that you'd give to Modify
func (f *Forest) BuildUndoData(numadds uint64, dels []uint64) *undoBlock {
	ub := new(undoBlock)
	ub.numAdds = uint32(numadds)

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
