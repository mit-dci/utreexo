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

// blockUndo is all the data needed to undo a block: number of adds,
// and all the hashes that got deleted.
type blockUndo struct {
	adds          uint32 // how many adds; chop this much off from the right
	deletedHashes []Hash // hashes that were overwritten or deleted
}

// Undo :
func (f *Forest) Undo(bu blockUndo) error {

	return nil
}
