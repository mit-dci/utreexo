package accumulator

// hashableNode is the data needed to perform a hash
type hashableNode struct {
	sib, dest *polNode
	position  uint64 // doesn't really need to be there, but convenient for debugging
}

type hashNpos struct {
	result Hash
	pos    uint64
}

func hashOne(l, r Hash, p uint64, hchan chan hashNpos) {
	var hnp hashNpos
	hnp.pos = p
	hnp.result = parentHash(l, r)
	hchan <- hnp
}

func (f *Forest) hashRow(dirtpositions []uint64) error {

	hchan := make(chan hashNpos, 256) // probably don't need that big a buffer

	for _, hp := range dirtpositions {
		l := f.data.read(child(hp, f.rows))
		r := f.data.read(child(hp, f.rows) | 1)
		// fmt.Printf("hash pos %d l %x r %x\n", hp, l[:4], r[:4])
		go hashOne(l, r, hp, hchan)
	}

	for remaining := len(dirtpositions); remaining > 0; remaining-- {
		hnp := <-hchan
		f.data.write(hnp.pos, hnp.result)
	}

	return nil
}
