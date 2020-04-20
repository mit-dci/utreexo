package util

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
)

// Hash represents a 256 bit sha256 hash
type Hash [32]byte

// Prefix is the first 4 bytes of Hash. It's meant for printfs
func (h Hash) Prefix() []byte {
	return h[:4]
}

// Mini takes a 32 byte hash slice and returns a MiniHash(first 12 bytes)
// Done for storage efficiency
func (h Hash) Mini() (m MiniHash) {
	copy(m[:], h[:12])
	return
}

// MiniHash is the first 12 byte chunk of a 32 byte hash of a leaf
// Done for storage efficiency
type MiniHash [12]byte

// HashFromString takes a string and returns a sha256 hash of that string
func HashFromString(s string) Hash {
	return sha256.Sum256([]byte(s))
}

// Arrow describes the movement of a node from one position to another
// used for transform
type Arrow struct {
	From, To uint64
}

// Arrowh is Arrow with height field added
type Arrowh struct {
	From, To uint64
	Ht       uint8
}

// Node :
type Node struct {
	Pos uint64
	Val Hash
}

// LeafTXOs have a hash and a expiry date (block when that utxo gets used)
type LeafTXO struct {
	Hash
	Duration int32
	// During ibdsim, this will dictate whether it is saved to
	// the memory or not.
	Remember bool // this leaf will be deleted soon, remember it
}

// Parent gets you the merkle parent. So far no committing to height.
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
	TtlSlices    [][]Hash
	BlockHeight  int32
	LeafCounter  uint64
	DurationMask uint32
	Lookahead    int32
}

// NewSimChain :
func NewSimChain(duration uint32) *SimChain {
	var s SimChain
	s.BlockHeight = -1
	s.DurationMask = duration
	s.TtlSlices = make([][]Hash, s.DurationMask+1)
	return &s
}

// BackOne takes the output of NextBlock and undoes the block
func (s *SimChain) BackOne(leaves []LeafTXO, dels []Hash) {

	// push in the deleted hashes on the left, trim the rightmost
	s.TtlSlices =
		append([][]Hash{dels}, s.TtlSlices[:len(s.TtlSlices)-1]...)

	// Gotta go through the leaves and delete them all from the ttlslices
	for _, l := range leaves {
		if l.Duration == 0 {
			continue
		}
		fmt.Printf("removing %x at end of row %d\n", l.Hash[:4], l.Duration)
		// everything should be in order, right?
		fmt.Printf("remove %x from end of ttl slice %d\n",
			s.TtlSlices[l.Duration][len(s.TtlSlices[l.Duration])-1][:4],
			l.Duration)
		s.TtlSlices[l.Duration] =
			s.TtlSlices[l.Duration][:len(s.TtlSlices[l.Duration])-1]
	}

	s.BlockHeight--
	return
}

func (s *SimChain) TtlString() string {
	x := "-------------\n"
	for i, d := range s.TtlSlices {
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
	s.BlockHeight++
	fmt.Printf("blockHeight %d\n", s.BlockHeight)

	if s.BlockHeight == 0 && numAdds == 0 {
		numAdds = 1
	}
	// they're all forgettable
	adds := make([]LeafTXO, numAdds)

	// make dels; dels are preset by the ttlMap
	delHashes := s.TtlSlices[0]
	s.TtlSlices = append(s.TtlSlices[1:], []Hash{})

	// make a bunch of unique adds & make an expiry time and add em to
	// the TTL map
	for j := range adds {
		adds[j].Hash[0] = uint8(s.LeafCounter)
		adds[j].Hash[1] = uint8(s.LeafCounter >> 8)
		adds[j].Hash[2] = uint8(s.LeafCounter >> 16)
		adds[j].Hash[3] = 0xff
		adds[j].Hash[4] = uint8(s.LeafCounter >> 24)
		adds[j].Hash[5] = uint8(s.LeafCounter >> 32)

		adds[j].Duration = int32(rand.Uint32() & s.DurationMask)
		// with "+1", the duration is 1 to 256, so the forest never gets
		// big or tall.  Without the +1, the duration is sometimes 0,
		// which makes a leaf last forever, and the forest will expand
		// over time.

		// the first utxo added lives forever.
		// (prevents leaves from going to 0 which is buggy)
		if s.BlockHeight == 0 {
			adds[j].Duration = 0
		}

		if adds[j].Duration != 0 && adds[j].Duration < s.Lookahead {
			adds[j].Remember = true
		}

		if adds[j].Duration != 0 {
			// fmt.Printf("put %x at row %d\n", adds[j].Hash[:4], adds[j].Duration-1)
			s.TtlSlices[adds[j].Duration-1] =
				append(s.TtlSlices[adds[j].Duration-1], adds[j].Hash)
		}

		s.LeafCounter++
	}

	return adds, delHashes
}
