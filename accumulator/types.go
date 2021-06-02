package accumulator

import (
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"math/rand"

	"github.com/mit-dci/utreexo/common"
)

// Hash is the 32 bytes of a sha256 hash
type Hash [32]byte

// Prefix for printfs
func (h Hash) Prefix() []byte {
	return h[:4]
}

// Mini takes the first 12 slices of a hash and outputs a MiniHash
func (h Hash) Mini() (m MiniHash) {
	copy(m[:], h[:12])
	return
}

// MiniHash is the first 12 bytes of a sha256 hash
type MiniHash [12]byte

// HashFromString takes a string and hashes with sha256
func HashFromString(s string) Hash {
	return sha256.Sum256([]byte(s))
}

// arrow describes the movement of a node from one position to another
type arrow struct {
	from, to uint64
	collapse bool
}

// node is an element in the utreexo tree and is represented by a position
// and a hash
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

// parentHash gets you the merkle parent of two children hashes.
// TODO So far no committing to height.
func parentHash(l, r Hash) Hash {
	var empty Hash
	if l == empty || r == empty {
		panic("got an empty leaf here. ")
	}
	buf := common.NewFreeBytes()
	defer buf.Free()
	buf.Bytes = append(buf.Bytes, l[:]...)
	buf.Bytes = append(buf.Bytes, r[:]...)
	return sha512.Sum512_256(buf.Bytes)
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

// NewSimChain initializes and returns a Simchain
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

// NextBlock outputs a new simulation block given the additions for the block
// to be outputed
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
