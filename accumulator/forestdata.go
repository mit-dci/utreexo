package accumulator

import (
	"bytes"
	"fmt"
	"os"
	"sort"
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
	close()
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

func (r *ramForestData) close() {
	// nothing to do here fro a ram forest.
}

// ********************************************* forest on disk
type diskForestCache struct {
	// The number of leaves contained in the cached part of the forest.
	Size uint64
	// The cache stores the forest data which is most frequently changed.
	// Based on the ttl distribution of bitcoin utxos.
	// (see figure 2 in the paper)
	data map[uint64]Hash
}

type cacheEntry struct {
	position uint64
	hash     Hash
}

type diskForestData struct {
	f *os.File
	// stores the size of the forest (the number of hashes stored).
	// gets updated on every size()/resize() call.
	hashCount uint64

	cache diskForestCache

	// for benchmarks:
	cacheReads  uint64
	cacheWrites uint64
	cacheMisses uint64

	diskReads  uint64
	diskWrites uint64
}

// Calculates the overlap of a range (start, start+r) with the cache.
// returns the amount of hashes of that range that are included in the cache.
func (cache diskForestCache) rangeOverlap(start, r, hashCount uint64) uint64 {
	row := uint8(0)
	rowOffset := uint64(0)

	cacheSize := cache.Size
	if cacheSize > hashCount {
		cacheSize = hashCount >> 1
	}

	for hashesCachedOnRow := cacheSize; hashesCachedOnRow>>row != 0; {
		totalHashesOnRow := hashCount >> (row + 1)
		minPosition := rowOffset + (totalHashesOnRow - hashesCachedOnRow)
		maxPosition := rowOffset + totalHashesOnRow

		if start < minPosition &&
			start+r >= minPosition {
			return (start + r) - minPosition
		}

		if start >= minPosition && start <= maxPosition {
			// The whole range lies with in the cache.
			return r
		}

		row++
		rowOffset += totalHashesOnRow
	}

	return 0
}

// Check if a position should be included in the cache based on `CacheSize`.
// Goes through each forest row and checks if `pos` is in the cached portion of that row.
func (cache diskForestCache) includes(pos uint64, hashCount uint64) bool {
	row := uint8(0)
	rowOffset := uint64(0)

	cacheSize := cache.Size
	if cacheSize > hashCount {
		cacheSize = hashCount >> 1
	}

	for hashesCachedOnRow := cacheSize; hashesCachedOnRow>>row != 0; {
		totalHashesOnRow := hashCount >> (row + 1)
		minPosition := rowOffset + (totalHashesOnRow - hashesCachedOnRow)
		maxPosition := rowOffset + totalHashesOnRow

		if pos >= minPosition && pos <= maxPosition {
			return true
		}
		row++
		rowOffset += totalHashesOnRow
	}

	return false
}

// Get a hash from the cache.
// Returns the hash found at pos and wether or not the cache was populated
// for that position. If it wasn't populated it should be with the contents
// from disk.
func (cache diskForestCache) get(pos uint64) (Hash, bool) {
	hash, ok := cache.data[pos]
	if !ok {
		// is hash==empty if ok==false?
		return empty, ok
	}

	return hash, ok
}

func (cache diskForestCache) rangeGet(start uint64, r uint64) ([]Hash, []uint64) {
	set := make([]Hash, r)
	var misses []uint64
	for i := uint64(0); i < r; i++ {
		hash, ok := cache.get(start + i)
		if !ok {
			misses = append(misses, i)
		}
		set[i] = hash
	}
	return set, misses
}

// Set a position in the cache.
// The previous value at that position is overwritten.
// Will create an entry in the cache wether
// or not it should actually be included.
// Check inclusion first with `includes`.
func (cache diskForestCache) set(pos uint64, hash Hash) {
	cache.data[pos] = hash
}

func (cache diskForestCache) rangeSet(start uint64,
	r uint64, hashes []Hash) {
	if r != uint64(len(hashes)) {
		panic(
			fmt.Sprintf(
				"rangeSet: range was %d but only %d hashes were given",
				r, len(hashes),
			),
		)
	}

	for i := uint64(0); i < r; i++ {
		cache.set(start+i, hashes[i])
	}
}

// Deletes all cache entries and returns them.
// Returned cache entries are sorted by their positions.
func (cache diskForestCache) flush() []*cacheEntry {
	cacheLength := len(cache.data)
	entries := make([]*cacheEntry, cacheLength)
	i := 0
	for pos, hash := range cache.data {
		entries[i] = &cacheEntry{
			position: pos,
			hash:     hash,
		}
		delete(cache.data, pos)
		i++
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].position < entries[j].position
	})

	return entries
}

// Deletes entries with positions that should not be
// in the cache after a resize.
// Returns deleted cache entries.
// Returned cache entries are sorted by their positions.
func (cache diskForestCache) flushOldHashes(
	newHashCount uint64) []*cacheEntry {
	var entries []*cacheEntry
	for pos, hash := range cache.data {
		if cache.includes(pos, newHashCount) {
			// Keep hashes in the cache that still are
			// in the cache after a resize.
			continue
		}
		entries = append(entries, &cacheEntry{
			position: pos,
			hash:     hash,
		})
		delete(cache.data, pos)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].position < entries[j].position
	})

	return entries
}

// read ignores errors. Probably get an empty hash if it doesn't work
func (d *diskForestData) read(pos uint64) Hash {
	var h Hash
	cacheMissed := false

	// Read `pos` from cache if the cache should include it.
	if d.cache.includes(pos, d.hashCount) {
		h, ok := d.cache.get(pos)
		if ok {
			// The cache did hold the value at `pos`.
			d.cacheReads++
			return h
		}
		// The cache did not hold the value at `pos`.
		cacheMissed = true
		d.cacheMisses++
	}

	// Read `pos` from disk.
	_, err := d.f.ReadAt(h[:], int64(pos*leafSize))
	if err != nil {
		fmt.Printf("\tWARNING!! read %x pos %d %s\n", h, pos, err.Error())
	}
	d.diskReads++

	if cacheMissed {
		// Populate the cache with the value read from disk.
		// On the next read of `pos` it will be fetched from the cache,
		// assuming the size of the forest doesn't change.
		// This is how the cache gets restored when the forest is restored from disk.
		d.cache.set(pos, h)
	}

	// `h` now holds the hash at `pos`, either read slowly from the disk
	// or fast from the cache.
	return h
}

// writeHash writes a hash.  Don't go out of bounds.
func (d *diskForestData) write(pos uint64, h Hash) {
	// Write `h` to `pos`in the cache if `pos` should be included in the cache.
	if d.cache.includes(pos, d.hashCount) {
		d.cache.set(pos, h)
		d.cacheWrites++
		return
	}

	// Write `h` to disk if it was not included in the cache.
	_, err := d.f.WriteAt(h[:], int64(pos*leafSize))
	if err != nil {
		fmt.Printf("\tWARNING!! write pos %d %s\n", pos, err.Error())
	}
	d.diskWrites++
}

// swapHash swaps 2 hashes.  Don't go out of bounds.
func (d *diskForestData) swapHash(a, b uint64) {
	ha := d.read(a)
	hb := d.read(b)
	d.write(a, hb)
	d.write(b, ha)
}

func (d *diskForestData) readRange(
	start, r uint64) (hashes []Hash) {
	// The number of hashes from the range included in the cache.
	cacheOverlap := d.cache.rangeOverlap(start, r, d.hashCount)
	// The number of hashes from the range stored on disk.
	diskOverlap := r - cacheOverlap
	diskPosition := int64(start * leafSize)

	// retrieve cache hashes.
	cacheHashes, misses := d.cache.rangeGet(start+diskOverlap, cacheOverlap)
	ok := len(misses) == 0
	if ok {
		d.cacheReads += cacheOverlap
	} else {
		// fetch misses from disk and populate cache.
		d.cacheMisses += uint64(len(misses))
		missBatchSize := 1
		batchPosition := misses[0]
		for i := uint64(0); i < uint64(len(misses)-1); i++ {
			miss := misses[i]
			nextMiss := misses[i+1]
			if miss == nextMiss+1 {
				// sequential misses can be batched.
				missBatchSize++
				continue
			}

			missBatch := make([]byte, missBatchSize*leafSize)
			_, err := d.f.ReadAt(missBatch,
				int64(uint64(diskPosition)+
					diskOverlap*uint64(leafSize)+
					batchPosition*uint64(leafSize)),
			)
			if err != nil {
				fmt.Printf("\treadRange WARNING!! read pos %d len %d %s\n (while populating cache)",
					diskPosition, diskOverlap, err.Error())
			}
			d.diskReads += uint64(missBatchSize)

			for j := uint64(0); j < uint64(missBatchSize); j++ {
				copy(cacheHashes[batchPosition+j][:], missBatch[:j*leafSize])
				d.cache.set(start+diskOverlap+miss, cacheHashes[batchPosition+j])
			}

			missBatchSize = 1
			batchPosition = nextMiss
		}
	}

	// retrieve disk hashes.
	hashes = make([]Hash, diskOverlap)
	diskRange := make([]byte, leafSize*diskOverlap)

	_, err := d.f.ReadAt(diskRange, diskPosition)
	if err != nil {
		fmt.Printf("\treadRange WARNING!! read pos %d len %d %s\n",
			diskPosition, diskOverlap, err.Error())
	}
	d.diskReads += diskOverlap

	// convert diskRange to diskHashes
	// TODO: this is ugly. we have 2 copies of the diskHashes in memory.
	for i := range hashes {
		copy(hashes[i][:], diskRange[i*leafSize:(i*leafSize)+32])
	}

	hashes = append(hashes, cacheHashes...)

	return
}

func (d *diskForestData) writeRange(
	start, r uint64, hashes []Hash) {
	cacheOverlap := d.cache.rangeOverlap(start, r, d.hashCount)
	diskOverlap := r - cacheOverlap
	hashIndex := uint64(0)

	// write disk hashes.
	var diskBuf bytes.Buffer
	for ; hashIndex < diskOverlap; hashIndex++ {
		diskBuf.Write(hashes[hashIndex][:])
	}

	diskPosition := int64(start * leafSize)
	_, err := d.f.WriteAt(diskBuf.Bytes(), diskPosition) // write arange to b
	if err != nil {
		fmt.Printf("\twriteRange WARNING!! read pos %d len %d %s\n",
			diskPosition, diskOverlap, err.Error())
	}
	d.diskWrites += diskOverlap

	// write cache hashes.
	d.cache.rangeSet(
		start+diskOverlap,
		cacheOverlap,
		hashes[diskOverlap:],
	)
	d.cacheWrites += cacheOverlap
}

// swapHashRange swaps 2 continuous ranges of hashes.  Don't go out of bounds.
// uses lots of ram to make only 3 disk seeks (depending on how you count? 4?)
// seek to a start, read a, seek to b start, read b, write b, seek to a, write a
// depends if you count seeking from b-end to b-start as a seek. or if you have
// like read & replace as one operation or something.
func (d *diskForestData) swapHashRange(a, b, w uint64) {
	hashesA := d.readRange(a, w)
	hashesB := d.readRange(b, w)
	d.writeRange(b, w, hashesA)
	d.writeRange(a, w, hashesB)
}

// size gives you the size of the forest
func (d *diskForestData) size() uint64 {
	s, err := d.f.Stat()
	if err != nil {
		fmt.Printf("\tWARNING: %s. Returning 0", err.Error())
		return 0
	}
	d.hashCount = uint64(s.Size() / leafSize)
	return d.hashCount
}

// resize makes the forest bigger (never gets smaller so don't try)
func (d *diskForestData) resize(newSize uint64) {
	fmt.Println("resize: ", newSize)
	cacheEntries := d.cache.flushOldHashes(newSize)
	err := d.f.Truncate(int64(newSize * leafSize))
	if err != nil {
		panic(err)
	}
	d.hashCount = newSize

	// write cache entries to disk.
	// TODO: batch write sequential entries.
	for _, entry := range cacheEntries {
		d.write(entry.position, entry.hash)
	}
}

func (d *diskForestData) close() {
	// flush the entire cache to disk.
	cacheEntries := d.cache.flush()
	// TODO: batch write sequential entries.
	for _, entry := range cacheEntries {
		d.write(entry.position, entry.hash)
	}

	fmt.Printf("forest data cache benchmarks:"+
		"cacheReads: %d, cacheWrites: %d, cacheMisses: %d, diskReads: %d, diskWrites: %d, hashCount: %d\n",
		d.cacheReads, d.cacheWrites, d.cacheMisses, d.diskReads, d.diskWrites, d.hashCount)
}
