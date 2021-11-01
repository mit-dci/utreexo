package accumulator

// hashableNode is the data needed to perform a hash
type hashableNode struct {
	sib, dest *polNode
	position  uint64 // doesn't really need to be there, but convenient for debugging
}

// hashRow calculates new hashes for all the positions passed in
func (f *Forest) hashRow(dirtpositions []uint64) []uint64 {

	// t := time.Now()

	// make wg
	// var wg sync.WaitGroup

	// wg.Add(len(dirtpositions))

	// for _, hp := range dirtpositions {

	// 	go func() {
	// 		// t1 := time.Now()
	// 		l := f.data.read(child(hp, f.rows))
	// 		r := f.data.read(child(hp, f.rows) | 1)
	// 		// fmt.Println("Reads take ", time.Since(t)) // each read takes from 42-207ns
	// 		f.data.write(hp, parentHash(l, r))
	// 		// t2 := time.Now()
	// 		// fmt.Println("Writes take", time.Since(t2)) // each write takes anything from 30-130ns
	// 		wg.Done()
	// 	}()
	// }

	// wg.Wait()

	// fmt.Println("Total time taken by I/O", time.Since(t))

	// return nil

	// for _, hp := range dirtpositions {
	// 	l := f.data.read(child(hp, f.rows))
	// 	r := f.data.read(child(hp, f.rows) | 1)
	// 	f.data.write(hp, parentHash(l, r))
	// }

	return nil

	// try
	var nextRow []uint64
	for _, dp := range dirtpositions {
		if dp&1 == 1 {
			l := f.data.read(child(dp, f.rows))
			r := f.data.read(child(dp, f.rows) | 1)
			pHash := parentHash(l, r)
			nextRow = append(nextRow, )
		}
	}
}
