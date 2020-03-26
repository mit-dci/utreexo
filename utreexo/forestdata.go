package utreexo

import (
	"fmt"
	"os"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// leafSize is a [32]byte hash (sha256).
// Length is always 32.
const leafSize = 32

// A forestData is the thing that holds all the hashes in the forest.  Could
// be in a file, or in ram, or maybe something else.
type ForestData interface {
	read(pos uint64) Hash
	write(pos uint64, h Hash)
	swapHash(a, b uint64)
	swapHashRange(a, b, w uint64)
	size() uint64
	resize(newSize uint64) // make it have a new size (bigger)
}

// ********************************************* forest in ram

type ramForestData struct {
	m []Hash
}

// TODO it reads a lot of empty locations which can't be good

// reads from specified location.  If you read beyond the bounds that's on you
// and it'll crash
func (r *ramForestData) read(pos uint64) Hash {
	// if r.m[pos] == empty {
	// 	fmt.Printf("\tuseless read empty at pos %d\n", pos)
	// }
	return r.m[pos]
}

// writeHash writes a hash.  Don't go out of bounds.
func (r *ramForestData) write(pos uint64, h Hash) {
	// if h == empty {
	// 	fmt.Printf("\tWARNING!! write empty at pos %d\n", pos)
	// }
	r.m[pos] = h
}

// TODO there's lots of empty writes as well, mostly in resize?  Anyway could
// be optimized away.

// swapHash swaps 2 hashes.  Don't go out of bounds.
func (r *ramForestData) swapHash(a, b uint64) {
	r.m[a], r.m[b] = r.m[b], r.m[a]
}

// swapHashRange swaps 2 continuous ranges of hashes.  Don't go out of bounds.
// could be sped up if you're ok with using more ram.
func (r *ramForestData) swapHashRange(a, b, w uint64) {
	// fmt.Printf("swaprange %d %d %d\t", a, b, w)
	for i := uint64(0); i < w; i++ {
		r.m[a+i], r.m[b+i] = r.m[b+i], r.m[a+i]
		// fmt.Printf("swapped %d %d\t", a+i, b+i)
	}

}

// size gives you the size of the forest
func (r *ramForestData) size() uint64 {
	return uint64(len(r.m))
}

// resize makes the forest bigger (never gets smaller so don't try)
func (r *ramForestData) resize(newSize uint64) {
	r.m = append(r.m, make([]Hash, newSize-r.size())...)
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
		fmt.Printf("\tWARNING!! read %x pos %d %s\n", h, pos, err.Error())
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

// swapHashRange swaps 2 continuous ranges of hashes.  Don't go out of bounds.
// uses lots of ram to make only 3 disk seeks (depending on how you count? 4?)
// seek to a start, read a, seek to b start, read b, write b, seek to a, write a
// depends if you count seeking from b-end to b-start as a seek. or if you have
// like read & replace as one operation or something.
func (d *diskForestData) swapHashRange(a, b, w uint64) {
	arange := make([]byte, 32*w)
	brange := make([]byte, 32*w)
	_, err := d.f.ReadAt(arange, int64(a*leafSize)) // read at a
	if err != nil {
		fmt.Printf("\tshr WARNING!! read pos %d len %d %s\n",
			a*leafSize, w, err.Error())
	}
	_, err = d.f.ReadAt(brange, int64(b*leafSize)) // read at b
	if err != nil {
		fmt.Printf("\tshr WARNING!! read pos %d len %d %s\n",
			b*leafSize, w, err.Error())
	}
	_, err = d.f.WriteAt(arange, int64(b*leafSize)) // write arange to b
	if err != nil {
		fmt.Printf("\tshr WARNING!! write pos %d len %d %s\n",
			b*leafSize, w, err.Error())
	}
	_, err = d.f.WriteAt(brange, int64(a*leafSize)) // write brange to a
	if err != nil {
		fmt.Printf("\tshr WARNING!! write pos %d len %d %s\n",
			a*leafSize, w, err.Error())
	}
}

// size gives you the size of the forest
func (d *diskForestData) size() uint64 {
	s, err := d.f.Stat()
	if err != nil {
		fmt.Printf("\tWARNING: %s. Returning 0", err.Error())
		return 0
	}
	return uint64(s.Size() / leafSize)
}

// resize makes the forest bigger (never gets smaller so don't try)
func (d *diskForestData) resize(newSize uint64) {
	err := d.f.Truncate(int64(newSize * leafSize))
	if err != nil {
		panic(err)
	}
}

// ForestPosition used to be postionMap.  It's not a map anymore.  Though
// it could be.  But probably need to use levelDB.
type ForestPosition interface {
	add(Hash, utxoData) // add hash & extra data
	rem(Hash)           // remove hash
	read(Hash) utxoData // read extra data for hash
	move(Hash, uint64)  // move position data for hash, leaving bytes
	end()
}

type utxoData struct {
	pos   uint64
	extra []byte // will change this to a real struct soon
	// TODO         ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
}

func (u *utxoData) toBytes() []byte {
	return append(U64tB(u.pos), u.extra...)
}

func utxoDataFromBytes(b []byte) utxoData {
	var u utxoData
	if len(b) < 8 {
		return u
	}
	u.pos = BtU64(b[:8])
	u.extra = b[8:]
	return u
}

type ramForestPostion struct {
	m map[MiniHash]utxoData
}

func startRamForestPosition() *ramForestPostion {
	r := new(ramForestPostion)
	r.m = make(map[MiniHash]utxoData)
	return r
}

func (r *ramForestPostion) add(h Hash, u utxoData) {
	r.m[h.Mini()] = u
}

func (r *ramForestPostion) rem(h Hash) {
	delete(r.m, h.Mini())
}

func (r *ramForestPostion) read(h Hash) utxoData {
	u, ok := r.m[h.Mini()]
	if !ok {
		return utxoData{pos: 1 << 60}
	}
	return u
}

func (r *ramForestPostion) move(h Hash, p uint64) {
	u := r.m[h.Mini()]
	u.pos = p
	r.m[h.Mini()] = u
}

func (r *ramForestPostion) end() {
}

type levelDBForestPostion struct {
	db *leveldb.DB
}

func startLevelDBForestPosition(folder string) *levelDBForestPostion {
	l := new(levelDBForestPostion)
	o := new(opt.Options)
	o.CompactionTableSizeMultiplier = 8
	// remove old folder if there
	err := os.RemoveAll(folder)
	if err != nil {
		panic(err)
	}
	// r := rand.Uint64()
	// randomizedPath := fmt.Sprintf("%s%x", folder, r)
	l.db, err = leveldb.OpenFile(folder, o)
	if err != nil {
		panic(err)
	}

	return l
}

func (l *levelDBForestPostion) add(h Hash, u utxoData) {
	err := l.db.Put(h[:], u.toBytes(), nil)
	if err != nil {
		fmt.Printf("add: levelDB error %s", err.Error())
	}
}

func (l *levelDBForestPostion) rem(h Hash) {
	err := l.db.Delete(h[:], nil)
	if err != nil {
		fmt.Printf("rem: levelDB error %s", err.Error())
	}
}

func (l *levelDBForestPostion) read(h Hash) utxoData {
	b, err := l.db.Get(h[:], nil)
	if err != nil {
		fmt.Printf("read: levelDB error %s", err.Error())
	}
	return utxoDataFromBytes(b)
}

func (l *levelDBForestPostion) move(h Hash, p uint64) {
	b, err := l.db.Get(h[:], nil)
	if err != nil {
		fmt.Printf("move get: levelDB error %s", err.Error())
	}
	b = append(U64tB(p), b[8:]...)
	err = l.db.Put(h[:], b, nil)
	if err != nil {
		fmt.Printf("move put: levelDB error %s", err.Error())
	}
}

func (l *levelDBForestPostion) end() {
	err := l.db.Close()
	if err != nil {
		fmt.Printf("end: levelDB error %s", err.Error())
	}
}
