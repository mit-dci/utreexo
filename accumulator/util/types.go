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
	// Hash represents the 32 byte SHA256 hash of a TXO
	Hash

	// Remember dictates whether the LeafTXO is saved to the cache
	// or not
	Remember bool
}

type simLeaf struct {
	LeafTXO
	duration int32
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

// SimChain is for testing purposes. Returns "blocks" of adds and deletes
type SimChain struct {
	// ttlMap is when the hashes get removed
	TTLSlices    [][]Hash
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
	s.TTLSlices = make([][]Hash, s.DurationMask+1)
	return &s
}

// BackOne takes the output of NextBlock and undoes the block
func (s *SimChain) BackOne(leaves []LeafTXO, durations []int32, dels []Hash) {

	// push in the deleted hashes on the left, trim the rightmost
	s.TTLSlices =
		append([][]Hash{dels}, s.TTLSlices[:len(s.TTLSlices)-1]...)

	// Gotta go through the leaves and delete them all from the ttlslices
	for i, l := range leaves {
		if durations[i] == 0 {
			continue
		}
		fmt.Printf("removing %x at end of row %d\n", l.Hash[:4], durations[i])
		// everything should be in order, right?
		fmt.Printf("remove %x from end of ttl slice %d\n",
			s.TTLSlices[durations[i]][len(s.TTLSlices[durations[i]])-1][:4],
			durations[i])
		s.TTLSlices[durations[i]] =
			s.TTLSlices[durations[i]][:len(s.TTLSlices[durations[i]])-1]
	}

	s.BlockHeight--
	return
}

func (s *SimChain) TTLString() string {
	x := "-------------\n"
	for i, d := range s.TTLSlices {
		x += fmt.Sprintf("%d: ", i)
		for _, h := range d {
			x += fmt.Sprintf(" %x ", h[:4])
		}
		x += fmt.Sprintf("\n")
	}

	return x
}

// NextBlock :
func (s *SimChain) NextBlock(numAdds uint32) ([]LeafTXO, []int32, []Hash) {
	s.BlockHeight++
	fmt.Printf("blockHeight %d\n", s.BlockHeight)

	if s.BlockHeight == 0 && numAdds == 0 {
		numAdds = 1
	}
	// they're all forgettable
	adds := make([]LeafTXO, numAdds)
	durations := make([]int32, numAdds)

	// make dels; dels are preset by the ttlMap
	delHashes := s.TTLSlices[0]
	s.TTLSlices = append(s.TTLSlices[1:], []Hash{})

	// make a bunch of unique adds & make an expiry time and add em to
	// the TTL map
	for j := range adds {
		adds[j].Hash[0] = uint8(s.LeafCounter)
		adds[j].Hash[1] = uint8(s.LeafCounter >> 8)
		adds[j].Hash[2] = uint8(s.LeafCounter >> 16)
		adds[j].Hash[3] = 0xff
		adds[j].Hash[4] = uint8(s.LeafCounter >> 24)
		adds[j].Hash[5] = uint8(s.LeafCounter >> 32)

		durations[j] = int32(rand.Uint32() & s.DurationMask)

		// with "+1", the duration is 1 to 256, so the forest never gets
		// big or tall.  Without the +1, the duration is sometimes 0,
		// which makes a leaf last forever, and the forest will expand
		// over time.

		// the first utxo added lives forever.
		// (prevents leaves from going to 0 which is buggy)
		if s.BlockHeight == 0 {
			durations[j] = 0
		}

		if durations[j] != 0 && durations[j] < s.Lookahead {
			adds[j].Remember = true
		}

		if durations[j] != 0 {
			// fmt.Printf("put %x at row %d\n", adds[j].Hash[:4], adds[j].duration-1)
			s.TTLSlices[durations[j]-1] =
				append(s.TTLSlices[durations[j]-1], adds[j].Hash)
		}

		s.LeafCounter++
	}

	return adds, durations, delHashes
}
