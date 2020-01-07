package utreexo

// A forestData is the thing that holds all the hashes in the forest.  Could
// be in a file, or in ram, or maybe something else.
type forestData interface {
	read(pos uint64) Hash
	write(pos uint64, h Hash)
	swapHash(a, b uint64)
	size() uint64
	resize(moreSize uint64)
}

type ramForestData struct {
	ramForest []Hash
}

// reads from specified location.  If you read beyond the bounds that's on you
// and it'll crash
func (r *ramForestData) read(pos uint64) Hash {
	return r.ramForest[pos]
}

// writeHash writes a hash.  Don't go out of bounds.
func (r *ramForestData) write(pos uint64, h Hash) {
	r.ramForest[pos] = h
}

// swapHash swaps 2 hashes.  Don't go out of bounds.
func (r *ramForestData) swapHash(a, b uint64) {
	r.ramForest[a], r.ramForest[b] = r.ramForest[b], r.ramForest[a]
}

// size gives you the size of the forest
func (r *ramForestData) size() uint64 {
	return uint64(len(r.ramForest))
}

// resize makes the forest bigger (never gets smaller so don't try)
func (r *ramForestData) resize(moreSize uint64) {
	r.ramForest = append(r.ramForest, make([]Hash, moreSize)...)
}
