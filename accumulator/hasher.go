package accumulator

import (
	"sync"
)

// hashableNode is the data needed to perform a hash
type hashableNode struct {
	sib, dest *polNode
	position  uint64 // doesn't really need to be there, but convenient for debugging
}

// this should work, right?  like the pointeryness?  Because swapnodes doesn't
// change pointers.  Well it changes niece pointers, but the data itself changes
// so the data is "there" in a static structure so I can use pointers to it
// and it won't move around.  hopefully.

func (n *hashableNode) run(wg *sync.WaitGroup) {
	// fmt.Printf("hasher about to replace %x\n", n.dest.data[:4])
	n.dest.safeAssignData(n.sib.auntOp())
	// n.dest.data = n.sib.auntOp()
	// fmt.Printf("hasher finished %x %x -> %x\n",
	// n.sib.niece[0].data[:4], n.sib.niece[1].data[:4], n.dest.data[:4])
	wg.Done()
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
