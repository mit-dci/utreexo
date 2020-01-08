package utreexo

import (
	"fmt"
	"os"
)

// leafSize is a [32]byte hash (sha256).
// Length is alwasy 32.
const leafSize = 32

// A forestData is the thing that holds all the hashes in the forest.  Could
// be in a file, or in ram, or maybe something else.
type ForestData interface {
	read(pos uint64) Hash
	write(pos uint64, h Hash)
	swapHash(a, b uint64)
	size() uint64
	resize(moreSize uint64)
}

// ********************************************* forest in ram

type ramForestData struct {
	m []Hash
}

// reads from specified location.  If you read beyond the bounds that's on you
// and it'll crash
func (r *ramForestData) read(pos uint64) Hash {
	return r.m[pos]
}

// writeHash writes a hash.  Don't go out of bounds.
func (r *ramForestData) write(pos uint64, h Hash) {
	r.m[pos] = h
}

// swapHash swaps 2 hashes.  Don't go out of bounds.
func (r *ramForestData) swapHash(a, b uint64) {
	r.m[a], r.m[b] = r.m[b], r.m[a]
}

// size gives you the size of the forest
func (r *ramForestData) size() uint64 {
	return uint64(len(r.m))
}

// resize makes the forest bigger (never gets smaller so don't try)
func (r *ramForestData) resize(moreSize uint64) {
	r.m = append(r.m, make([]Hash, moreSize)...)
}

// ********************************************* forest on disk
type diskForestData struct {
	f *os.File
}

// read ignores errors. Probably get an empty hash if it doesn't work
func (d *diskForestData) read(pos uint64) Hash {
	var h Hash
	_, err := d.f.ReadAt(h[:], int64(pos*leafSize))
	if err != nil {
		fmt.Printf("\tWARNING!! read pos %d %s\n", pos, err.Error())
	}
	return h
}

// writeHash writes a hash.  Don't go out of bounds.
func (d *diskForestData) write(pos uint64, h Hash) {
	_, err := d.f.WriteAt(h[:], int64(pos*leafSize))
	if err != nil {
		fmt.Printf("\tWARNING!! write pos %d %s\n", pos, err.Error())
	}
}

// swapHash swaps 2 hashes.  Don't go out of bounds.
func (d *diskForestData) swapHash(a, b uint64) {
	ha := d.read(a)
	hb := d.read(b)
	d.write(a, hb)
	d.write(b, ha)
}

// size gives you the size of the forest
func (d *diskForestData) size() uint64 {
	s, err := d.f.Stat()
	if err != nil {
		fmt.Errorf("\tWARNING: %s. Returning 0", err.Error())
		return 0
	}
	return uint64(s.Size())
}

// resize makes the forest bigger (never gets smaller so don't try)
func (d *diskForestData) resize(moreSize uint64) {
	d.f.Truncate(int64(d.size() + moreSize))
}
