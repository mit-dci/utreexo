package accumulator

import (
	"encoding/binary"
	"fmt"
	"io"
)

/* we need to be able to undo blocks!  for bridge nodes at least.
compact nodes can just keep old roots.
although actually it can make sense for non-bridge nodes to undo as well...
*/

// TODO in general, deal with numLeaves going to 0

// blockUndo is all the data needed to undo a block: number of adds,
// and all the hashes that got deleted and where they were from
type UndoBlock struct {
	Height    int32    // height of block
	numAdds   uint32   // number of adds in the block
	positions []uint64 // position of all deletions this block
	hashes    []Hash   // hashes that were deleted
}

// ToString returns a string
func (u *UndoBlock) ToString() string {
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

// SerializeSize returns how many bytes it would take to serialize this undoblock.
func (u *UndoBlock) SerializeSize() int {
	// Size of u.numAdds + len(u.positions) + each position takes up 8 bytes
	size := 4 + 8 + (len(u.positions) * 8)
	// Size of len(u.hashes) + each hash takes up 32 bytes
	size += 8 + (len(u.hashes) * 32)

	return size
}

// Serialize encodes the undoblock into the given writer.
func (u *UndoBlock) Serialize(w io.Writer) error {
	err := binary.Write(w, binary.BigEndian, u.numAdds)
	if err != nil {
		return err
	}

	err = binary.Write(w, binary.BigEndian, uint64(len(u.positions)))
	if err != nil {
		return err
	}

	err = binary.Write(w, binary.BigEndian, u.positions)
	if err != nil {
		return err
	}

	err = binary.Write(w, binary.BigEndian, uint64(len(u.hashes)))
	if err != nil {
		return err
	}

	for _, hash := range u.hashes {
		n, err := w.Write(hash[:])
		if err != nil {
			return err
		}

		if n != 32 {
			err := fmt.Errorf("UndoBlock Serialize supposed to write 32 bytes but wrote %d bytes", n)
			return err
		}
	}

	return nil
}

// Deserialize decodes an undoblock from the reader.
func (u *UndoBlock) Deserialize(r io.Reader) error {
	err := binary.Read(r, binary.BigEndian, &u.numAdds)
	if err != nil {
		return err
	}

	var posCount uint64
	err = binary.Read(r, binary.BigEndian, &posCount)
	if err != nil {
		return err
	}
	u.positions = make([]uint64, posCount)

	err = binary.Read(r, binary.BigEndian, u.positions)
	if err != nil {
		return err
	}

	var hashCount uint64
	err = binary.Read(r, binary.BigEndian, &hashCount)
	if err != nil {
		return err
	}

	u.hashes = make([]Hash, hashCount)

	for i := uint64(0); i < hashCount; i++ {
		n, err := r.Read(u.hashes[i][:])
		if err != nil {
			return err
		}

		if n != 32 {
			err := fmt.Errorf("UndoBlock Deserialize supposed to read 32 bytes but read %d bytes", n)
			return err
		}
	}

	return nil
}
