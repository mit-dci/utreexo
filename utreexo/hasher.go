package utreexo

import (
	"fmt"
	"sync"
)

/*
the idea behind hasher: it's a routine that accepts polNode pointers with
a 'higher' bool.  It maintains internal state where it only starts hashing row 3
when it's done with all of row 2.  So whoever is throwing work at it, here
are the rules--
you can send work for 2, 2, 2, 2, 3, 3, 3 and don't have to wait for 2 to
finish before sending 3.  BUT!  don't send 2, 2, 3, 3, 2, 2 because the hasher
may begin work on 3 without the work on 2 ready.
You don't actually send the height, you just send a bool, "higher" on the
FIRST node of the new height, so in this case the first at height 3.

It's just a buffered channel; if it's not done with the 2s by the time
3s start coming in, the 3s will just pile up in the buffer and then start
being removed once all the 2s are done.  In theory it would still work with a
tiny buffer (or even none?) but if hashing is much slower than the swapping
operations, you could have all the things to hash in the buffer before a single
hash finishes, so it's good to have a big buffer.

There's 2 waitgroups: one given when you call hasher, and one used internally
to tick between rows.  Wait on the extrnal one to make sure everything's done.

*/

// hasher is the routine that accepts hashableNodeWithHeights and does em all
// It doesn't give an indication that it's finished, but if you've sent
func hasher(hashChan chan hashableNode, hold chan bool, extWg *sync.WaitGroup) {
	rowWg := new(sync.WaitGroup)
	for {
		select {
		case <-hold: // if we get a hold signal
			rowWg.Wait() // wait until prev row is finished before continuing
		default:
		}

		incoming := <-hashChan // grab hashable
		fmt.Printf("hasher got %x %x\n", incoming.l[:4], incoming.r[:4])
		rowWg.Add(1)                  // add to row waitgroup
		go incoming.run(rowWg, extWg) // hand off to hashing
	}
}

// hashableNode is the data needed to perform a hash
type hashableNode struct {
	l, r, p *Hash
}

// this should work, right?  like the pointeryness?  Because swapnodes doesn't
// change pointers.  Well it changes niece pointers, but the data itself changes
// so the data is "there" in a static structure so I can use pointers to it
// and it won't move around.  hopefully.

func (n *hashableNode) run(rwg, xwg *sync.WaitGroup) {
	fmt.Printf("hasher finished %x %x %x\n", n.l[:4], n.r[:4], n.p[:4])
	*n.p = Parent(*n.l, *n.r)
	fmt.Printf("hasher finished %x %x %x\n", n.l[:4], n.r[:4], n.p[:4])
	rwg.Done()
	xwg.Done()
}
