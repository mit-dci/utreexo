package accumulator

import (
	"fmt"
	"os"
)

// ********************************************* forest on disk with cache
type diskForestCache struct {
	// The number of leaves contained in the cached part of the forest.
	size uint64
	// `valid` stores which positions are set in the cache.
	valid []bool
	// The cache stores the forest data which is most frequently changed.
	// Based on the ttl distribution of bitcoin utxos.
	// (see figure 2 in the paper)
	data []byte
}

// creates a new cache.
func newDiskForestCache(trees uint64) *diskForestCache {
	size := uint64(1 << trees)
	fmt.Printf("newDiskForestCache: forest data cache size is set to %dMB\n",
		((size<<1) /*valid*/ +(size<<1)*leafSize /*data*/)>>20)

	return &diskForestCache{
		size:  size,
		valid: make([]bool, size<<1),
		data:  make([]byte, (size<<1)*leafSize),
	}
}

type cacheRange struct {
	// the start position of this range in the cache
	startCache uint64
	// the start position of this range in the forest
	start uint64
	// the amount of hashes in the range
	count uint64
}

type cacheForestData struct {
	file *os.File
	// stores the size of the forest (the number of hashes stored).
	// gets updated on every size()/resize() call.
	hashCount uint64

	cache *diskForestCache
}

// Calculates the overlap of a range (start, start+r) with the cache.
// returns the amount of hashes of that range that are included in the cache.
func (cache *diskForestCache) rangeOverlap(
	start, r, hashCount uint64) (uint64, uint64) {
	row := uint8(0)
	rowOffset := uint64(0)

	cacheSize := cache.size
	if cacheSize > hashCount>>1 {
		cacheSize = hashCount >> 1
	}

	hashesNotCached := uint64(0)
	for hashesCachedOnRow := cacheSize; hashesCachedOnRow != 0; hashesCachedOnRow >>= 1 {
		totalHashesOnRow := hashCount >> (row + 1)
		hashesNotCached += (totalHashesOnRow - hashesCachedOnRow)

		minPosition := rowOffset + (totalHashesOnRow - hashesCachedOnRow)
		maxPosition := rowOffset + totalHashesOnRow - 1

		if start < minPosition &&
			start+r >= minPosition {
			return (start + r) - minPosition, (start + r) - hashesNotCached
		}

		if start >= minPosition && start <= maxPosition {
			// The whole range lies with in the cache.
			return r, start - hashesNotCached
		}

		row++
		rowOffset += totalHashesOnRow
	}

	return 0, 0
}

// Check if a position should be included in the cache based on `cache.Size`.
// Goes through each forest row and checks if `pos` is in the cached portion of that row.
func (cache *diskForestCache) includes(
	pos uint64, hashCount uint64) (included bool, cachePosition uint64) {
	row := uint8(0)
	rowOffset := uint64(0)

	cacheSize := cache.size
	if cacheSize > hashCount>>1 {
		cacheSize = hashCount >> 1
	}

	hashesNotCached := uint64(0)
	for hashesCachedOnRow := cacheSize; hashesCachedOnRow != 0; hashesCachedOnRow >>= 1 {
		totalHashesOnRow := hashCount >> (row + 1)
		hashesNotCached += (totalHashesOnRow - hashesCachedOnRow)

		minPosition := rowOffset + (totalHashesOnRow - hashesCachedOnRow)
		maxPosition := rowOffset + totalHashesOnRow - 1

		if pos >= minPosition && pos <= maxPosition {
			included = true
			cachePosition = pos - hashesNotCached
			return
		}
		row++
		rowOffset += totalHashesOnRow
	}

	included = false
	cachePosition = 0
	return
}

// Get a hash from the cache.
// Returns the hash found at `pos` and wether or not the cache was populated
// at that position. If it wasn't populated it should be with the contents
// from disk.
// `pos` must be a cache position returned from `includes`.
func (cache *diskForestCache) get(pos uint64) (Hash, bool) {
	populated := cache.valid[pos]
	if !populated {
		return empty, false
	}

	var h Hash
	copy(h[:], cache.data[pos*leafSize:(pos+1)*leafSize])

	return h, true
}

// Gets a range of hashes.
// Returns the hashes as a byte slice and unpopulated cache positions relative to `start`.
func (cache *diskForestCache) rangeGet(start uint64, r uint64) ([]byte, []uint64) {
	var misses []uint64
	for check := uint64(0); check < r; check++ {
		if !cache.valid[check+start] {
			misses = append(misses, check)
		}
	}

	set := make([]byte, r*leafSize)
	copy(set, cache.data[start*leafSize:(start+r)*leafSize])

	return set, misses
}

// Set a position in the cache.
// The previous value at that position is overwritten.
// Will create an entry in the cache wether
// or not it should actually be included.
// Check inclusion first with `includes`.
func (cache *diskForestCache) set(pos uint64, hash []byte) {
	copy(cache.data[pos*leafSize:(pos+1)*leafSize], hash)
	cache.valid[pos] = true
}

func (cache *diskForestCache) rangeSet(start uint64,
	r uint64, hashes []byte) {
	if r != uint64(len(hashes)>>5 /*divided by leafSize*/) {
		panic(
			fmt.Sprintf(
				"rangeSet: range was %d but only %d hashes were given",
				r, len(hashes)/leafSize,
			),
		)
	}

	for populate := start; populate < start+r; populate++ {
		// mark all entries in the range as populated
		cache.valid[populate] = true
	}

	copy(cache.data[start*leafSize:(start+r)*leafSize], hashes[:r*leafSize])
}

// Resets the cache and returns populated cache ranges.
// sort of expensive but not needed often.
func (cache *diskForestCache) flush(hashCount uint64) []cacheRange {
	cacheLength := cache.size << 1
	var entries []cacheRange

	row := uint8(0)
	rowOffset := uint64(0)

	cacheSize := cache.size
	if cacheSize > hashCount>>1 {
		cacheSize = hashCount >> 1
	}

	hashesNotCached := uint64(0)
	for hashesCachedOnRow := cacheSize; hashesCachedOnRow != 0; hashesCachedOnRow >>= 1 {
		totalHashesOnRow := hashCount >> (row + 1)
		minPosition := rowOffset + (totalHashesOnRow - hashesCachedOnRow)
		hashesNotCached += (totalHashesOnRow - hashesCachedOnRow)

		cachePosition := minPosition - hashesNotCached
		count := uint64(0)
		for i := uint64(0); i < hashesCachedOnRow+1; i++ {
			// if the end of the row is reached
			if i == hashesCachedOnRow ||
				// or the cache is not populated at `cachePosition+i`.
				!cache.valid[cachePosition+i] {
				// append the range of populated entries.
				entries = append(entries, cacheRange{
					start:      minPosition + i - count,
					startCache: cachePosition + i - count,
					count:      count,
				})
				// reset the count
				count = 0
				continue
			}

			count++
		}

		row++
		rowOffset += totalHashesOnRow
	}

	// reset the populated map
	cache.valid = make([]bool, cacheLength)

	return entries
}

// read ignores errors. Probably get an empty hash if it doesn't work
func (d *cacheForestData) read(pos uint64) Hash {
	var h Hash
	inCache, cachePos := d.cache.includes(pos, d.hashCount)
	cacheMissed := false

	// Read `pos` from cache if the cache should include it.
	if inCache {
		h, ok := d.cache.get(cachePos)
		if ok {
			// The cache did hold the value at `pos`.
			return h
		}
		// The cache did not hold the value at `pos`.
		cacheMissed = true
	}

	// Read `pos` from disk.
	_, err := d.file.ReadAt(h[:], int64(pos*leafSize))
	if err != nil {
		fmt.Printf("\tWARNING!! read %x pos %d %s\n", h, pos, err.Error())
	}

	if cacheMissed {
		// Populate the cache with the value read from disk.
		// On the next read of `pos` it will be fetched from the cache,
		// assuming the size of the forest doesn't change.
		// This is how the cache gets restored when the forest is restored from disk.
		d.cache.set(cachePos, h[:])
	}

	// `h` now holds the hash at `pos`, either read slowly from the disk
	// or fast from the cache.
	return h
}

// writeHash writes a hash.  Don't go out of bounds.
func (d *cacheForestData) write(pos uint64, h Hash) {
	inCache, cachePos := d.cache.includes(pos, d.hashCount)

	// Write `h` to `pos` in the cache if `pos` should be included in the cache.
	if inCache {
		d.cache.set(cachePos, h[:])
		return
	}

	// Write `h` to disk if it was not included in the cache.
	_, err := d.file.WriteAt(h[:], int64(pos*leafSize))
	if err != nil {
		fmt.Printf("\tWARNING!! write pos %d %s\n", pos, err.Error())
	}
}

// swapHash swaps 2 hashes.  Don't go out of bounds.
func (d *cacheForestData) swapHash(a, b uint64) {
	ha := d.read(a)
	hb := d.read(b)
	d.write(a, hb)
	d.write(b, ha)
}

// read a range from the forest.
// reads from cache and disk.
func (d *cacheForestData) readRange(
	start, r uint64) (hashes []byte) {
	// The number of hashes from the range included in the cache.
	cacheOverlap, cacheStart := d.cache.rangeOverlap(start, r, d.hashCount)
	// The number of hashes from the range stored on disk.
	diskOverlap := r - cacheOverlap
	diskPosition := int64(start * leafSize)

	cacheHashes, misses := d.cache.rangeGet(cacheStart, cacheOverlap)

	if len(misses) != 0 {
		// Some entries were not in the cache and should be read from disk.
		for _, miss := range misses {
			diskPosition := int64((diskOverlap + miss + start) * leafSize)
			// TODO: batch read for sequential misses.
			_, err := d.file.ReadAt(cacheHashes[miss*leafSize:(miss+1)*leafSize], diskPosition)
			if err != nil {
				fmt.Printf("\tWARNING!! read pos %d %s\n", start, err.Error())
			}
		}
	}

	hashes = make([]byte, leafSize*diskOverlap)
	_, err := d.file.ReadAt(hashes, diskPosition)
	if err != nil {
		fmt.Printf("\tWARNING!! read pos %d %s\n", start, err.Error())
	}

	hashes = append(hashes, cacheHashes...)
	return
}

// write a range to the forest data.
// Writes to the cache and the disk.
func (d *cacheForestData) writeRange(
	start, r uint64, hashes []byte) {
	// calculate the cacheOverlap for the range
	cacheOverlap, cacheStart := d.cache.rangeOverlap(start, r, d.hashCount)
	diskOverlap := r - cacheOverlap
	diskPosition := int64(start * leafSize)

	// write the cacheoverlap of the range to the cache.
	d.cache.rangeSet(cacheStart, cacheOverlap, hashes[diskOverlap*leafSize:])

	// write the diskoverlap of the range to disk
	_, err := d.file.WriteAt(
		hashes[:diskOverlap*leafSize],
		diskPosition,
	)
	if err != nil {
		fmt.Printf("\tWARNING!! write pos %d %s\n", diskPosition, err.Error())
	}
}

// swapHashRange swaps 2 continuous ranges of hashes.  Don't go out of bounds.
// uses lots of ram to make only 3 disk seeks (depending on how you count? 4?)
// seek to a start, read a, seek to b start, read b, write b, seek to a, write a
// depends if you count seeking from b-end to b-start as a seek. or if you have
// like read & replace as one operation or something.
func (d *cacheForestData) swapHashRange(a, b, w uint64) {
	hashesA := d.readRange(a, w)
	hashesB := d.readRange(b, w)
	d.writeRange(b, w, hashesA)
	d.writeRange(a, w, hashesB)
}

// size gives you the size of the forest
func (d *cacheForestData) size() uint64 {
	s, err := d.file.Stat()
	if err != nil {
		fmt.Printf("\tWARNING: %s. Returning 0", err.Error())
		return 0
	}
	d.hashCount = uint64(s.Size() / leafSize)
	return d.hashCount
}

// resize makes the forest bigger (never gets smaller so don't try)
func (d *cacheForestData) resize(newSize uint64) {
	err := d.file.Truncate(int64(newSize * leafSize))
	if err != nil {
		panic(err)
	}

	flushCacheToDisk(d)

	d.hashCount = newSize
}

func (d *cacheForestData) close() {
	flushCacheToDisk(d)
}

func flushCacheToDisk(d *cacheForestData) {
	// flush the entire cache to disk.
	cacheRanges := d.cache.flush(d.hashCount)
	// write cache entries to disk.
	for _, r := range cacheRanges {
		// write to disk
		_, err := d.file.WriteAt(
			d.cache.data[r.startCache*leafSize:(r.startCache+r.count)*leafSize],
			int64(r.start*leafSize),
		)
		if err != nil {
			fmt.Printf("\tWARNING!! write pos %d %s\n", r.start, err.Error())
		}
	}
}
