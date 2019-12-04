package utreexo

import (
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
func hasher(hashChan chan hashableNode, higher chan bool, extWg sync.WaitGroup) {
	var rowWg sync.WaitGroup
	for {

		incoming := <-hashChan

		rowWg.Add(1)
		go incoming.run(rowWg, extWg)

		select {
		case <-higher:
			rowWg.Wait()
		default:
		}

	}
}

type hashableNode struct {
	l, r, p *Hash
}

func (n *hashableNode) run(wga, wgb sync.WaitGroup) {
	*n.p = Parent(*n.l, *n.r)
	wga.Done()
	wgb.Done()
}
