package accumulator

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Pollard is the sparse representation of the utreexo forest, using
// binary tree pointers instead of a hash map.

// I generally avoid recursion as much as I can, using regular for loops and
// ranges instead.  That might start looking pretty contrived here, but
// I'm still going to try it.

// Pollard :
type Pollard struct {
	numLeaves uint64 // number of leaves in the pollard forest

	roots []polNode // slice of the tree roots, which are polNodes.
	// roots are in big to small order
	// BUT THEY'RE WEIRD!  The left / right children are actual children,
	// not nieces as they are in every lower level.

	hashesEver, rememberEver, overWire uint64

	Lookahead int32 // remember leafs below this TTL
	//	Minleaves uint64 // remember everything below this leaf count

	positionMap map[MiniHash]uint64
}

// PolNode is a node in the pollard forest
type polNode struct {
	data  Hash
	niece [2]*polNode
}

// auntOp returns the hash of a nodes nieces. crashes if you call on nil nieces.
func (n *polNode) auntOp() Hash {
	return parentHash(n.niece[0].data, n.niece[1].data)
}

// auntable tells you if you can call auntOp on a node
func (n *polNode) auntable() bool {
	return n.niece[0] != nil && n.niece[1] != nil
}

// deadEnd returns true if both nieces are nill
// could also return true if n itself is nil! (maybe a bad idea?)
func (n *polNode) deadEnd() bool {
	// if n == nil {
	// 	fmt.Printf("nil deadend\n")
	// 	return true
	// }
	return n.niece[0] == nil && n.niece[1] == nil
}

// chop turns a node into a deadEnd by setting both nieces to nil.
func (n *polNode) chop() {
	n.niece[0] = nil
	n.niece[1] = nil
}

//  printout printfs the node
func (n *polNode) printout() {
	if n == nil {
		fmt.Printf("nil node\n")
		return
	}
	fmt.Printf("Node %x ", n.data[:4])
	if n.niece[0] == nil {
		fmt.Printf("l nil ")
	} else {
		fmt.Printf("l %x ", n.niece[0].data[:4])
	}
	if n.niece[1] == nil {
		fmt.Printf("r nil\n")
	} else {
		fmt.Printf("r %x\n", n.niece[1].data[:4])
	}
}

// prune prunes deadend children.
// don't prune at the bottom; use leaf prune instead at row 1
func (n *polNode) prune() {
	if n.niece[0].deadEnd() {
		n.niece[0] = nil
	}
	if n.niece[1].deadEnd() {
		n.niece[1] = nil
	}
}

// polSwap swaps the contents of two polNodes & leaves pointers to them intact
// need their siblings so that the siblings' nieces can swap.
// for a root, just say the root's sibling is itself and it should work.
func polSwap(a, asib, b, bsib *polNode) error {
	if a == nil || asib == nil || b == nil || bsib == nil {
		return fmt.Errorf("polSwap given nil node")
	}
	a.data, b.data = b.data, a.data
	asib.niece, bsib.niece = bsib.niece, asib.niece
	return nil
}

func (p *Pollard) rows() uint8 { return treeRows(p.numLeaves) }

// rootHashesReverse is ugly and returns the root hashes in reverse order
// ... which is the order full forest is using until I can refactor that code
// to make it big to small order
func (p *Pollard) rootHashesReverse() []Hash {
	rHashes := make([]Hash, len(p.roots))
	for i, n := range p.roots {
		rHashes[len(rHashes)-(1+i)] = n.data
	}
	return rHashes
}

//  ------------------ pollard serialization
// currently saving /restoring pollard to disk only does the roots.
// so you lose all the caching
// TODO have the option to save restore sparse pollards.  Could use the same
// idea as verifyBatchProof

// current serialization is just 8byte numleaves, followed by all the hashes
// (in small to big order)

func (p *Pollard) WritePollard(w io.Writer) error {
	var err error
	err = binary.Write(w, binary.BigEndian, p.numLeaves)
	if err != nil {
		return err
	}
	for _, t := range p.roots {
		_, err = w.Write(t.data[:])
		if err != nil {
			return err
		}
	}
	fmt.Println("Pollard leaves:", p.numLeaves)
	return nil
}

func (p *Pollard) RestorePollard(r io.Reader) error {
	fmt.Println("Restoring Pollard Roots...")
	err := binary.Read(r, binary.BigEndian, &p.numLeaves)
	if err != nil {
		return err
	}

	p.roots = make([]polNode, numRoots(p.numLeaves))
	fmt.Printf("%d leaves %d roots ", p.numLeaves, len(p.roots))
	for i, _ := range p.roots {
		bytesRead, err := r.Read(p.roots[i].data[:])
		// ignore EOF error at the end of successful reading
		if err != nil && !(bytesRead == 32 && err == io.EOF) {
			fmt.Printf("on hash %d read %d ", i, bytesRead)
			return err
		}
	}
	fmt.Println("Finished restoring pollard")
	return nil
}
