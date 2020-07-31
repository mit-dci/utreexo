package accumulator

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
)

// Hash :
type Hash [32]byte

// Prefix for printfs
func (h Hash) Prefix() []byte {
	return h[:4]
}

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

// arrow describes the movement of a node from one position to another
type arrow struct {
	from, to uint64
}

// Node :
type node struct {
	Pos uint64
	Val Hash
}

// Leaf contains a hash and a hint about whether it should be saved to
// memory or not during ibdsim.
type Leaf struct {
	Hash
	Remember bool // this leaf will be deleted soon, remember it
}

type simLeaf struct {
	Leaf
	duration int32
}

// Parent gets you the merkle parent.  So far no committing to height.
// if the left child is zero it should crash...
func parentHash(l, r Hash) Hash {
	var empty Hash
	if l == empty || r == empty {
		panic("got an empty leaf here. ")
	}
	return sha256.Sum256(append(l[:], r[:]...))
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
func (s *SimChain) BackOne(leaves []Leaf, durations []int32, dels []Hash) {

	// push in the deleted hashes on the left, trim the rightmost
	s.ttlSlices =
		append([][]Hash{dels}, s.ttlSlices[:len(s.ttlSlices)-1]...)

	// Gotta go through the leaves and delete them all from the ttlslices
	for i, l := range leaves {
		if durations[i] == 0 {
			continue
		}
		fmt.Printf("removing %x at end of row %d\n", l.Hash[:4], durations[i])
		// everything should be in order, right?
		fmt.Printf("remove %x from end of ttl slice %d\n",
			s.ttlSlices[durations[i]][len(s.ttlSlices[durations[i]])-1][:4],
			durations[i])
		s.ttlSlices[durations[i]] =
			s.ttlSlices[durations[i]][:len(s.ttlSlices[durations[i]])-1]
	}

	s.blockHeight--
}

func (s *SimChain) ttlString() string {
	x := "-------------\n"
	for i, d := range s.ttlSlices {
		x += fmt.Sprintf("%d: ", i)
		for _, h := range d {
			x += fmt.Sprintf(" %x ", h[:4])
		}
		x += "\n"
	}

	return x
}

// NextBlock :
func (s *SimChain) NextBlock(numAdds uint32) ([]Leaf, []int32, []Hash) {
	s.blockHeight++
	fmt.Printf("blockHeight %d\n", s.blockHeight)

	if s.blockHeight == 0 && numAdds == 0 {
		numAdds = 1
	}
	// they're all forgettable
	adds := make([]Leaf, numAdds)
	durations := make([]int32, numAdds)

	// make dels; dels are preset by the ttlMap
	delHashes := s.ttlSlices[0]
	s.ttlSlices = append(s.ttlSlices[1:], []Hash{})

	// make a bunch of unique adds & make an expiry time and add em to
	// the TTL map
	for j, _ := range adds {
		adds[j].Hash[0] = uint8(s.leafCounter)
		adds[j].Hash[1] = uint8(s.leafCounter >> 8)
		adds[j].Hash[2] = uint8(s.leafCounter >> 16)
		adds[j].Hash[3] = 0xff
		adds[j].Hash[4] = uint8(s.leafCounter >> 24)
		adds[j].Hash[5] = uint8(s.leafCounter >> 32)

		durations[j] = int32(rand.Uint32() & s.durationMask)

		// with "+1", the duration is 1 to 256, so the forest never gets
		// big or tall.  Without the +1, the duration is sometimes 0,
		// which makes a leaf last forever, and the forest will expand
		// over time.

		// the first utxo added lives forever.
		// (prevents leaves from going to 0 which is buggy)
		if s.blockHeight == 0 {
			durations[j] = 0
		}

		if durations[j] != 0 && durations[j] < s.lookahead {
			adds[j].Remember = true
		}

		if durations[j] != 0 {
			// fmt.Printf("put %x at row %d\n", adds[j].Hash[:4], adds[j].duration-1)
			s.ttlSlices[durations[j]-1] =
				append(s.ttlSlices[durations[j]-1], adds[j].Hash)
		}

		s.leafCounter++
	}

	return adds, durations, delHashes
}
