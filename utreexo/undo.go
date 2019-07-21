package utreexo

/* we need to be able to undo blocks!  for bridge nodes at least.
compact nodes can just keep old roots.
although actually it can make sense for non-bridge nodes to undo as well...
*/

//
type undo struct {
	move // move the hash at from to position to
	Hash // replace the hash at from with this
}

type blockUndo struct {
	adds  uint32 // how many adds; chop this much off from the right
	undos []undo
}

// GetTops returns all the tops of the trees
func (f *Forest) Undo(bu blockUndo) error {

	return nil
}
