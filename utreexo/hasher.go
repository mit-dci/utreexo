package utreexo

import (
	"fmt"
	"sync"
)

// hashableNode is the data needed to perform a hash
type hashableNode struct {
	l, r, p *Hash
}

// this should work, right?  like the pointeryness?  Because swapnodes doesn't
// change pointers.  Well it changes niece pointers, but the data itself changes
// so the data is "there" in a static structure so I can use pointers to it
// and it won't move around.  hopefully.

func (n *hashableNode) run(wg *sync.WaitGroup) {
	*n.p = Parent(*n.l, *n.r)
	fmt.Printf("hasher finished %x %x %x\n", n.l[:4], n.r[:4], n.p[:4])
	wg.Done()
}
