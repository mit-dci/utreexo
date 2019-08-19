package utreexo

import (
	"math/rand"

	"golang.org/x/crypto/blake2b"
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
	return blake2b.Sum256([]byte(s))
}

// an arror describes the movement of a node from one position to another
type arrow struct {
	from, to uint64
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
	return blake2b.Sum256(append(l[:], r[:]...))
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
	ttlMap       map[int32][]Hash
	blockHeight  int32
	leafCounter  uint64
	durationMask uint32
	lookahead    int32
}

// NewSimChain :
func NewSimChain() *SimChain {
	var s SimChain
	s.ttlMap = make(map[int32][]Hash)
	s.blockHeight = -1
	return &s
}

// NextBlock :
func (s *SimChain) NextBlock(numAdds uint32) ([]LeafTXO, []Hash) {
	s.blockHeight++
	b := s.blockHeight
	// they're all forgettable
	adds := make([]LeafTXO, numAdds)
	// make a bunch of unique adds & make an expiry time and add em to
	// the TTL map
	for j := range adds {
		adds[j].Hash[0] = uint8(s.leafCounter)
		adds[j].Hash[1] = uint8(s.leafCounter >> 8)
		adds[j].Hash[2] = uint8(s.leafCounter >> 16)
		adds[j].Hash[3] = uint8(s.leafCounter >> 24)
		adds[j].Hash[4] = uint8(s.leafCounter >> 32)
		adds[j].Hash[5] = 0xff

		duration := int32(rand.Uint32() & s.durationMask)
		// with "+1", the duration is 1 to 256, so the forest never gets
		// big or tall.  Without the +1, the duration is sometimes 0,
		// which makes a leaf last forever, and the forest will expand
		// over time.

		// the first utxo addded lives forever.
		// (prevents leaves from goign to 0 which is buggy)

		//		if s.blockHeight == 1 && j == 0 {
		//			adds[j].Duration = 0
		//		}

		if duration != 0 && duration < s.lookahead {
			adds[j].Remember = true
		}

		if duration != 0 {
			s.ttlMap[b+duration] = append(s.ttlMap[b+duration], adds[j].Hash)
		}

		s.leafCounter++
	}

	// make dels; dels are preset by the ttlMap
	delHashes := s.ttlMap[b]
	delete(s.ttlMap, b)

	return adds, delHashes
}
