package utreexo

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
)

// Hash :
type Hash [32]byte

// Mini :
func (h Hash) Mini() (m MiniHash) {
	copy(m[:], h[:12])
	return
}

// MiniHash :
type MiniHash [12]byte

// HashFromString :
func HashFromString(s string) Hash {
	return sha256.Sum256([]byte(s))
}

// an arror describes the movement of a node from one position to another
type arrow struct {
	from, to uint64
}

type arrowh struct {
	from, to uint64
	ht       uint8
}

// Node :
type Node struct {
	Pos uint64
	Val Hash
}

// LeafTXO 's have a hash and a expiry date (block when that utxo gets used)
type LeafTXO struct {
	Hash
	Duration int32
	Remember bool // this leaf will be deleted soon, remember it
}

// Parent gets you the merkle parent.  So far no committing to height.
// if the left child is zero it should crash...
func Parent(l, r Hash) Hash {
	var empty [32]byte
	if l == empty {
		panic("got a left empty here. ")
	}
	if r == empty {
		panic("got a right empty here. ")
	}
	return sha256.Sum256(append(l[:], r[:]...))
}

// XORParent is just xor, it's faster and works the same if non-adversarial
func xParent(l, r Hash) Hash {
	var x [32]byte
	if l == x {
		panic("got a left empty here. ")
	}
	if r == x {
		panic("got a right empty here. ")
	}

	for i := range l {
		x[i] = l[i] ^ r[i]
	}
	// just xor, it's faster and works the same if just testing

	return x
}

// SimChain is for testing; it spits out "blocks" of adds and deletes
type SimChain struct {
	// ttlMap is when the hashes get removed
	ttlSlices    [][]Hash
	blockHeight  int32
	leafCounter  uint64
	durationMask uint32
	lookahead    int32
}

// NewSimChain :
func NewSimChain(duration uint32) *SimChain {
	var s SimChain
	s.blockHeight = -1
	s.durationMask = duration
	s.ttlSlices = make([][]Hash, s.durationMask+1)
	return &s
}

// BackOne takes the output of NextBlock and undoes the block
func (s *SimChain) BackOne(leaves []LeafTXO, dels []Hash) {

	// push in the deleted hashes on the left, trim the rightmost
	s.ttlSlices =
		append([][]Hash{dels}, s.ttlSlices[:len(s.ttlSlices)-1]...)

	// Gotta go through the leaves and delete them all from the ttlslices
	for _, l := range leaves {
		if l.Duration == 0 {
			continue
		}
		fmt.Printf("removing %x at end of row %d\n", l.Hash[:4], l.Duration)
		// everything should be in order, right?
		fmt.Printf("remove %x from end of ttl slice %d\n",
			s.ttlSlices[l.Duration][len(s.ttlSlices[l.Duration])-1][:4],
			l.Duration)
		s.ttlSlices[l.Duration] =
			s.ttlSlices[l.Duration][:len(s.ttlSlices[l.Duration])-1]
	}

	s.blockHeight--
	return
}

func (s *SimChain) ttlString() string {
	x := "-------------\n"
	for i, d := range s.ttlSlices {
		x += fmt.Sprintf("%d: ", i)
		for _, h := range d {
			x += fmt.Sprintf(" %x ", h[:4])
		}
		x += fmt.Sprintf("\n")
	}

	return x
}

// NextBlock :
func (s *SimChain) NextBlock(numAdds uint32) ([]LeafTXO, []Hash) {
	s.blockHeight++
	fmt.Printf("blockHeight %d\n", s.blockHeight)

	if s.blockHeight == 0 && numAdds == 0 {
		numAdds = 1
	}
	// they're all forgettable
	adds := make([]LeafTXO, numAdds)

	// make dels; dels are preset by the ttlMap
	delHashes := s.ttlSlices[0]
	s.ttlSlices = append(s.ttlSlices[1:], []Hash{})

	// make a bunch of unique adds & make an expiry time and add em to
	// the TTL map
	for j := range adds {
		adds[j].Hash[0] = uint8(s.leafCounter)
		adds[j].Hash[1] = uint8(s.leafCounter >> 8)
		adds[j].Hash[2] = uint8(s.leafCounter >> 16)
		adds[j].Hash[3] = uint8(s.leafCounter >> 24)
		adds[j].Hash[4] = uint8(s.leafCounter >> 32)
		adds[j].Hash[5] = 0xff

		adds[j].Duration = int32(rand.Uint32() & s.durationMask)
		// with "+1", the duration is 1 to 256, so the forest never gets
		// big or tall.  Without the +1, the duration is sometimes 0,
		// which makes a leaf last forever, and the forest will expand
		// over time.

		// the first utxo addded lives forever.
		// (prevents leaves from goign to 0 which is buggy)

		if s.blockHeight == 0 {
			adds[j].Duration = 0
		}

		if adds[j].Duration != 0 && adds[j].Duration < s.lookahead {
			adds[j].Remember = true
		}

		if adds[j].Duration != 0 {
			fmt.Printf("put %x at row %d\n", adds[j].Hash[:4], adds[j].Duration-1)
			s.ttlSlices[adds[j].Duration-1] =
				append(s.ttlSlices[adds[j].Duration-1], adds[j].Hash)
		}

		s.leafCounter++
	}

	return adds, delHashes
}
