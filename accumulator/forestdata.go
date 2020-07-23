package accumulator

import (
	"fmt"
	"os"
)

// leafSize is a [32]byte hash (sha256).
// Length is always 32.
const leafSize = 32

// ForestData is the thing that holds all the hashes in the forest.  Could
// be in a file, or in ram, or maybe something else.
type ForestData interface {
	read(pos uint64) Hash
	write(pos uint64, h Hash)
	swapHash(a, b uint64)
	swapHashRange(a, b, w uint64)
	size() uint64
	resize(newSize uint64) // make it have a new size (bigger)
	close()
}

// ********************************************* forest in ram

type ramForestData struct {
	// m []Hash
	m []byte
}

// TODO it reads a lot of empty locations which can't be good

// reads from specified location.  If you read beyond the bounds that's on you
// and it'll crash
func (r *ramForestData) read(pos uint64) (h Hash) {
	// if r.m[pos] == empty {
	// 	fmt.Printf("\tuseless read empty at pos %d\n", pos)
	// }
	pos <<= 5
	copy(h[:], r.m[pos:pos+32])
	return
}

// writeHash writes a hash.  Don't go out of bounds.
func (r *ramForestData) write(pos uint64, h Hash) {
	// if h == empty {
	// 	fmt.Printf("\tWARNING!! write empty at pos %d\n", pos)
	// }
	pos <<= 5
	copy(r.m[pos:pos+32], h[:])
}

// TODO there's lots of empty writes as well, mostly in resize?  Anyway could
// be optimized away.

// swapHash swaps 2 hashes.  Don't go out of bounds.
func (r *ramForestData) swapHash(a, b uint64) {
	r.swapHashRange(a, b, 1) // just calls swap range..
}

// swapHashRange swaps 2 continuous ranges of hashes.  Don't go out of bounds.
// fast but uses more ram
func (r *ramForestData) swapHashRange(a, b, w uint64) {
	// fmt.Printf("swaprange %d %d %d\t", a, b, w)
	a <<= 5
	b <<= 5
	w <<= 5
	temp := make([]byte, w)
	copy(temp[:], r.m[a:a+w])

	copy(r.m[a:a+w], r.m[b:b+w])
	copy(r.m[b:b+w], temp[:])
}

// size gives you the size of the forest
func (r *ramForestData) size() uint64 {
	return uint64(len(r.m) / 32)
}

// resize makes the forest bigger (never gets smaller so don't try)
func (r *ramForestData) resize(newSize uint64) {
	r.m = append(r.m, make([]byte, (newSize-r.size())*leafSize)...)
}

func (r *ramForestData) close() {
	// nothing to do here fro a ram forest.
}

// ********************************************* forest on disk
type diskForestData struct {
	file *os.File
}

// read ignores errors. Probably get an empty hash if it doesn't work
func (d *diskForestData) read(pos uint64) Hash {
	var h Hash
	_, err := d.file.ReadAt(h[:], int64(pos*leafSize))
	if err != nil {
		fmt.Printf("\tWARNING!! read %x pos %d %s\n", h, pos, err.Error())
	}
	return h
}

// writeHash writes a hash.  Don't go out of bounds.
func (d *diskForestData) write(pos uint64, h Hash) {
	_, err := d.file.WriteAt(h[:], int64(pos*leafSize))
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

// swapHashRange swaps 2 continuous ranges of hashes.  Don't go out of bounds.
// uses lots of ram to make only 3 disk seeks (depending on how you count? 4?)
// seek to a start, read a, seek to b start, read b, write b, seek to a, write a
// depends if you count seeking from b-end to b-start as a seek. or if you have
// like read & replace as one operation or something.
func (d *diskForestData) swapHashRange(a, b, w uint64) {
	arange := make([]byte, leafSize*w)
	brange := make([]byte, leafSize*w)
	_, err := d.file.ReadAt(arange, int64(a*leafSize)) // read at a
	if err != nil {
		fmt.Printf("\tshr WARNING!! read pos %d len %d %s\n",
			a*leafSize, w, err.Error())
	}
	_, err = d.file.ReadAt(brange, int64(b*leafSize)) // read at b
	if err != nil {
		fmt.Printf("\tshr WARNING!! read pos %d len %d %s\n",
			b*leafSize, w, err.Error())
	}
	_, err = d.file.WriteAt(arange, int64(b*leafSize)) // write arange to b
	if err != nil {
		fmt.Printf("\tshr WARNING!! write pos %d len %d %s\n",
			b*leafSize, w, err.Error())
	}
	_, err = d.file.WriteAt(brange, int64(a*leafSize)) // write brange to a
	if err != nil {
		fmt.Printf("\tshr WARNING!! write pos %d len %d %s\n",
			a*leafSize, w, err.Error())
	}
}

// size gives you the size of the forest
func (d *diskForestData) size() uint64 {
	s, err := d.file.Stat()
	if err != nil {
		fmt.Printf("\tWARNING: %s. Returning 0", err.Error())
		return 0
	}
	return uint64(s.Size() / leafSize)
}

// resize makes the forest bigger (never gets smaller so don't try)
func (d *diskForestData) resize(newSize uint64) {
	err := d.file.Truncate(int64(newSize * leafSize * 2))
	if err != nil {
		panic(err)
	}
}

func (d *diskForestData) close() {
	err := d.file.Close()
	if err != nil {
		fmt.Printf("diskForestData close error: %s\n", err.Error())
	}
}
