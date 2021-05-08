package main

import (
	"math/rand"
	"testing"
)

// getSimCBlocks makes a slice of random cblocks.  THe TTLs will be random
// but never extend past the end of the cblock slice.  eg if you request 10
// cblocks, the 6th block will have a max ttl of 4

func getSimCBlocks(count int32) ([]cBlock, int) {
	cblocks := make([]cBlock, count)
	var totalTTLs int

	for h, _ := range cblocks {
		// uniform 0 to 100 TTLs per block
		cblocks[h].blockHeight = int32(h)
		cblocks[h].ttls = make([]int32, rand.Int31n(100))
		totalTTLs += len(cblocks[h].ttls)

		// TODO: make this a power law distribution for TTLs instead of
		// uniform up to max
		for i, _ := range cblocks[h].ttls {
			cblocks[h].ttls[i] = rand.Int31n(count - int32(h))
			if cblocks[h].ttls[i] == 0 {
				cblocks[h].ttls[i] = (1 << 31) - 1
			}
		}
	}
	return cblocks, totalTTLs
}

func TestSimCblocks(t *testing.T) {
	// cb50 := getSimCBlocks(50)

	// for _, cb := range cb50 {
	// fmt.Printf("block height %d ttls: %v\n", cb.blockHeight, cb.ttls)
	// }
}
