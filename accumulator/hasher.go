package accumulator

// hashableNode is the data needed to perform a hash
type hashableNode struct {
	sib, dest *polNode
	position  uint64 // doesn't really need to be there, but convenient for debugging
}

// hashRow calculates new hashes for all the positions passed in
func (f *Forest) hashRow(dirtpositions []uint64) error {
	for _, hp := range dirtpositions {
		l := f.data.read(child(hp, f.rows))
		r := f.data.read(child(hp, f.rows) | 1)
		f.data.write(hp, parentHash(l, r))
	}

	return nil
}

// hashdirt takes in dirt from revmovev5
// dirt positions should be in order, but not on
// the same row
func (f *Forest) hashDirt5(dirt []uint64) error {
	return nil
}
