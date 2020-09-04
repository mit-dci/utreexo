package accumulator

import (
	"testing"
)

func TestCacheSim(t *testing.T) {
	// New simulation chain with a lookahead cache of 32 blocks
	chain := NewSimChain(64 - 1)
	chain.lookahead = 32

	// New cache simulator for the pollard
	cacheSimulator := NewCacheSimulator(0)

	// Empty forest and pollard
	forest := NewForest(nil, false)
	var pollard Pollard

	for i := 0; i < 64; i++ {
		adds, _, delHashes := chain.NextBlock(100)

		proof, err := forest.ProveBatch(delHashes)
		if err != nil {
			t.Fatal("ProveBatch failed", err)
		}
		proof.SortTargets()
		_, err = forest.Modify(adds, proof.Targets)
		if err != nil {
			t.Fatal("Modify failed", err)
		}
		remember := make([]bool, len(adds))
		for i, add := range adds {
			remember[i] = add.Remember
		}

		proofPositions, _ := ProofPositions(proof.Targets, pollard.numLeaves, pollard.rows())
		// Run the simulator to retrieve the positions of the partial proof.
		neededPositions := cacheSimulator.Simulate(proof.Targets, remember)

		// check that the size of the partial proof is actually smaller than a regular proof.
		// if the partial proof is bigger it's not partial.
		if len(neededPositions) > len(proof.Proof) {
			t.Fatal("more positions needed than regular proof")
		}

		// check that the partial proof is minimal by ensuring that all the `neededPositions` are not cached.
		for _, pos := range neededPositions {
			n, _, _, _ := pollard.readPos(pos)
			if n != nil {
				t.Fatal("partial proof is not minimal. position", pos, "is cached but included in the partial proof")
			}
		}

		// check that all the positions that the simulator claims to be cached are actually cached.
		cached := sortedUint64SliceDiff(
			mergeSortedSlices(proofPositions, proof.Targets), neededPositions)
		for _, pos := range cached {
			n, _, _, err := pollard.readPos(pos)
			if err != nil || n == nil || n.data == empty {
				t.Fatal("simulated cache claimed to have", pos, "but did not", err)
			}
		}
		err = pollard.IngestBatchProof(proof)
		if err != nil {
			t.Fatal("IngestBatchProof failed", err)
		}
		err = pollard.Modify(adds, proof.Targets)
		if err != nil {
			t.Fatal("Modify failed", err)
		}
	}
}
