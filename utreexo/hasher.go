package utreexo

import (
	"fmt"
	"sync"
)

// hashableNode is the data needed to perform a hash
type hashableNode struct {
	sib, dest *polNode
}

// this should work, right?  like the pointeryness?  Because swapnodes doesn't
// change pointers.  Well it changes niece pointers, but the data itself changes
// so the data is "there" in a static structure so I can use pointers to it
// and it won't move around.  hopefully.

func (n *hashableNode) run(wg *sync.WaitGroup) {
	fmt.Printf("hasher about to replace %x\n", n.dest.data[:4])
	n.dest.data = n.sib.auntOp()
	fmt.Printf("hasher finished %x %x -> %x\n",
		n.sib.niece[0].data[:4], n.sib.niece[1].data[:4], n.dest.data[:4])
	wg.Done()
}
