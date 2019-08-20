package utreexo

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"os"
	"reflect"
)

// Pollard is the sparse representation of the utreexo forest, using
// binary tree pointers instead of a hash map.

// I generally avoid recursion as much as I can, using regular for loops and
// ranges instead.  That might start looking pretty contrived here, but
// I'm still going to try it.

// Pollard :
type Pollard struct {
	numLeaves uint64 // number of leaves in the pollard forest

	tops []*polNode // slice of the tree tops, which are polNodes.
	// tops are in big to small order
	// BUT THEY'RE WEIRD!  The left / right children are actual children,
	// not neices as they are in every lower level.

	hashesEver, rememberEver, overWire uint64

	//	Lookahead int32  // remember leafs below this TTL
	//	Minleaves uint64 // remember everything below this leaf count
}

// PolNode is a node in the pollard forest
type polNode struct {
	data  Hash
	niece [2]*polNode
}

// auntOp returns the hash of a nodes neices. crashes if you call on nil neices.
func (n *polNode) auntOp() Hash {
	return Parent(n.niece[0].data, n.niece[1].data)
}

// auntOp tells you if you can call auntOp on a node
func (n *polNode) auntable() bool {
	return n.niece[0] != nil && n.niece[1] != nil
}

// deadEnd returns true if both neices are nill
// could also return true if n itself is nil! (maybe a bad idea?)
func (n *polNode) deadEnd() bool {
	// if n == nil {
	// 	fmt.Printf("nil deadend\n")
	// 	return true
	// }
	return n.niece[0] == nil && n.niece[1] == nil
}

// chop turns a node into a deadEnd
func (n *polNode) chop() {
	n.niece[0] = nil
	n.niece[1] = nil
}

// prune prunes deadend children.
// don't prune at the bottom; use leaf prune instead at height 1
func (n *polNode) prune() {
	if n.niece[0].deadEnd() {
		n.niece[0] = nil
	}
	if n.niece[1].deadEnd() {
		n.niece[1] = nil
	}
}

// leafPrune is the prune method for leaves.  You don't want to chop off a leaf
// just because it's not memorable; it might be there because its sibling is
// memorable.  Call this at height 1 (not 0)
func (n *polNode) leafPrune() {
	if n.niece[0] != nil && n.niece[1] != nil &&
		n.niece[0].deadEnd() && n.niece[1].deadEnd() {
		n.chop()
	}
}

func (p *Pollard) height() uint8 { return treeHeight(p.numLeaves) }

// TopHashesReverse is ugly and returns the top hashes in reverse order
// ... which is the order full forest is using until I can refactor that code
// to make it big to small order
func (p *Pollard) topHashesReverse() []Hash {
	rHashes := make([]Hash, len(p.tops))
	for i, n := range p.tops {
		rHashes[len(rHashes)-(1+i)] = n.data
	}
	return rHashes
}

// store : Store Pollard to the file.
func (p *Pollard) store(path string) error {
	// TODO: Need to lock update
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	bufSize := 10240 // 10MB
	w := bufio.NewWriterSize(file, bufSize)
	// Write the number of leaves.
	num := make([]byte, 8)
	binary.LittleEndian.PutUint64(num, p.numLeaves)
	w.Write(num)
	w.Flush()
	// Write the hash values.
	_, hs := getTopsReverse(p.numLeaves, p.height())
	for i, top := range p.tops {
		h := hs[len(hs)-i-1]
		// If h is 0, it must be the last.
		if h == 0 {
			w.Write(top.data[:])
			break
		}
		// To restore, use Pollard#addone.
		// Therefore, the order of taking Hash is special.
		loop := 1 << (h - 1)
		for j := loop - 1; j >= 0; j-- {
			node := top
			for k := uint8(1); k < h; k++ {
				node = node.niece[(j>>(h-k-1))&1]
			}
			w.Write(node.niece[0].data[:])
			w.Write(node.niece[1].data[:])
		}
	}
	// Write the top hash values.
	for _, top := range p.tops {
		w.Write(top.data[:])
	}
	w.Flush() // Error handling necessary?
	return nil
}

// restore : Restore from the file to Pollard.
func restore(path string) (*Pollard, error) {
	file, err := os.OpenFile(path, os.O_RDONLY, 0644)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	// Restore the number of leaves.
	num := make([]byte, 8)
	_, err = file.Read(num)
	if err != nil {
		return nil, err
	}
	numLeaves := binary.LittleEndian.Uint64(num)
	// Check file size.
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	pc := PopCount(numLeaves)
	if info.Size() != 8+int64(numLeaves)*32+int64(pc)*32 {
		return nil, fmt.Errorf("illegal file size")
	}
	// Restore a Pollard.
	p := &Pollard{}
	for i := uint64(0); i < numLeaves; i++ {
		h := Hash{}
		_, err := file.Read(h[:])
		if err != nil {
			return nil, err
		}
		p.addOne(h, true)
	}
	// Get tops from the file.
	tops := []Hash{}
	for i := uint8(0); i < pc; i++ {
		top := Hash{}
		_, err = file.Read(top[:])
		if err != nil {
			return nil, err
		}
		tops = append(tops, top)
	}
	// Check tops.
	for i := range p.tops {
		if !reflect.DeepEqual(p.tops[i].data, tops[i]) {
			return nil, fmt.Errorf("unmatch top")
		}
	}
	return p, nil
}
