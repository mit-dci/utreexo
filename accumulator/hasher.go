package accumulator

import (
	"sync"
)

// hashableNode is the data needed to perform a hash
type hashableNode struct {
	sib, dest *polNode
	position  uint64 // doesn't really need to be there, but convenient for debugging
}

// hashRow calculates new hashes for all the positions passed in
func (f *Forest) hashRow(dirtpositions []uint64) []uint64 {

	// make wg
	var wg sync.WaitGroup

	wg.Add(len(dirtpositions))

	for _, hp := range dirtpositions {

		go func() {
			l := f.data.read(child(hp, f.rows))
			r := f.data.read(child(hp, f.rows) | 1)
			f.data.write(hp, parentHash(l, r))
			wg.Done()
		}()
	}

	wg.Wait()

	return nil

	// for _, hp := range dirtpositions {
	// 	l := f.data.read(child(hp, f.rows))
	// 	r := f.data.read(child(hp, f.rows) | 1)
	// 	f.data.write(hp, parentHash(l, r))
	// }

	// return nil
}
