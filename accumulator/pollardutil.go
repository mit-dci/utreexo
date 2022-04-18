package accumulator

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
)

// PolNode is a node in the pollard forest
type polNode struct {
	data       Hash
	leftNiece  *polNode
	rightNiece *polNode

	aunt     *polNode
	remember bool
}

// auntOp returns the hash of a nodes nieces. crashes if you call on nil nieces.
func (n *polNode) auntOp() Hash {
	return parentHash(n.leftNiece.data, n.rightNiece.data)
}

// auntable tells you if you can call auntOp on a node
func (n *polNode) auntable() bool {
	return n.leftNiece != nil && n.rightNiece != nil
}

// deadEnd returns true if both nieces are nil
// could also return true if n itself is nil! (maybe a bad idea?)
func (n *polNode) deadEnd() bool {
	// if n == nil {
	// 	fmt.Printf("nil deadend\n")
	// 	return true
	// }
	return n.leftNiece == nil && n.rightNiece == nil
}

// chop turns a node into a deadEnd by setting both nieces to nil.
func (n *polNode) chop() {
	n.leftNiece = nil
	n.rightNiece = nil
}

//  printout printfs the node
func (n *polNode) printout() {
	if n == nil {
		fmt.Printf("nil node\n")
		return
	}
	fmt.Printf("Node %x ", n.data[:4])
	if n.leftNiece == nil {
		fmt.Printf("l nil ")
	} else {
		fmt.Printf("l %x ", n.leftNiece.data[:4])
	}
	if n.rightNiece == nil {
		fmt.Printf("r nil\n")
	} else {
		fmt.Printf("r %x\n", n.rightNiece.data[:4])
	}
}

// PruneAll prunes the accumulator down to the roots.
func (p *Pollard) PruneAll() {
	for _, root := range p.roots {
		root.chop()
	}
}

// NumLeaves returns the number of leaves that the accumulator has.
func (p *Pollard) NumLeaves() uint64 {
	return p.numLeaves
}

func (p *Pollard) NumDels() uint64 {
	return p.numDels
}

// prune prunes deadend children.
// don't prune at the bottom; use leaf prune instead at row 1
func (n *polNode) prune() {
	remember := n.leftNiece.remember || n.rightNiece.remember
	if n.leftNiece.deadEnd() && !remember {
		n.leftNiece = nil
	}
	if n.rightNiece.deadEnd() && !remember {
		n.rightNiece = nil
	}
}

// getCount returns the count of all the nieces below it and itself.
func getCount(n *polNode) int64 {
	if n == nil {
		return 0
	}
	return (getCount(n.leftNiece) + 1 + getCount(n.rightNiece))
}

// polSwap swaps the contents of two polNodes & leaves pointers to them intact
// need their siblings so that the siblings' nieces can swap.
// for a root, just say the root's sibling is itself and it should work.
func polSwap(a, asib, b, bsib *polNode) error {
	if a == nil || asib == nil || b == nil || bsib == nil {
		return fmt.Errorf("polSwap given nil node")
	}
	a.data, b.data = b.data, a.data
	//asib.niece, bsib.niece = bsib.niece, asib.niece
	asib.leftNiece, bsib.leftNiece = bsib.leftNiece, asib.leftNiece
	asib.rightNiece, bsib.rightNiece = bsib.rightNiece, asib.rightNiece
	a.remember, b.remember = b.remember, a.remember
	return nil
}
func (p *Pollard) rows() uint8 { return treeRows(p.numLeaves) }

// rootHashesForward grabs the rootHashes from left to right
func (p *Pollard) rootHashesForward() []Hash {
	rHashes := make([]Hash, len(p.roots))
	for i, n := range p.roots {
		rHashes[i] = n.data
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

// WritePollard writes the numLeaves field and only the roots into the given writer.
// Cached leaves are not included in the writer
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
	return nil
}

// RestorePollard restores the pollard from the given reader
func (p *Pollard) RestorePollard(r io.Reader) error {
	err := binary.Read(r, binary.BigEndian, &p.numLeaves)
	if err != nil {
		return err
	}

	p.roots = make([]*polNode, numRoots(p.numLeaves))
	fmt.Printf("%d leaves %d roots ", p.numLeaves, len(p.roots))
	for i, _ := range p.roots {
		p.roots[i] = new(polNode)
		bytesRead, err := r.Read(p.roots[i].data[:])
		// ignore EOF error at the end of successful reading
		if err != nil && !(bytesRead == 32 && err == io.EOF) {
			s := fmt.Errorf("err: %v on hash %d read %d", err, i, bytesRead)
			return s
		}
	}
	return nil
}

// Serialize serializes the numLeaves field and only the roots into a byte slice.
// Cached leaves are not included in the byte slice
func (p *Pollard) Serialize() ([]byte, error) {
	size := 8 + len(p.roots) // 8 for uint64 numLeaves
	serialized := make([]byte, 0, size)

	buf := bytes.NewBuffer(serialized)

	err := binary.Write(buf, binary.BigEndian, p.numLeaves)
	if err != nil {
		return nil, err
	}

	for _, t := range p.roots {
		_, err = buf.Write(t.data[:])
		if err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

// Deserialize decodes the bytes into a Pollard
func (p *Pollard) Deserialize(serialized []byte) error {
	reader := bytes.NewReader(serialized)

	err := binary.Read(reader, binary.BigEndian, &p.numLeaves)
	if err != nil {
		return err
	}
	fmt.Println(p.numLeaves)

	p.roots = make([]*polNode, numRoots(p.numLeaves))

	for i, _ := range p.roots {
		p.roots[i] = new(polNode)
		bytesRead, err := reader.Read(p.roots[i].data[:])

		// ignore EOF error at the end of successful reading
		if err != nil && !(bytesRead == 32 && err == io.EOF) {
			s := fmt.Errorf("err: %v on hash %d read %d", err, i, bytesRead)
			return s
		}
	}

	return nil
}

// PrintRemembers prints all the nodes and their remember status.  Useful for debugging.
func (p *Pollard) PrintRemembers() (string, error) {
	str := ""

	rows := p.rows()
	// very naive loop looping outside the edge of the tree
	for i := uint64(0); i < (2<<rows)-1; i++ {
		// check if the leaf is within the tree
		if !inForest(i, p.numLeaves, rows) {
			continue
		}
		n, _, _, err := p.readPos(i)
		if err != nil {
			return "", err
		}
		if n != nil {
			str += fmt.Sprintf("pos %d, data %s, remember %v\n",
				i, hex.EncodeToString(n.data[:]), n.remember)
		}
	}

	return str, nil
}
