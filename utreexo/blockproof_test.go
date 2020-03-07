package utreexo

import "testing"

// TestVerifyBlockProof tests that the computedTop is compared to the top in the
// Utreexo forest.
func TestVerifyBlockProof(t *testing.T) {
	// Create forest in memory
	f := NewForest(nil)

	// last index to be deleted. Same as blockDels
	lastIdx := uint64(7)

	// Generate adds
	adds := make([]LeafTXO, 8)
	adds[0].Hash = Hash{1}
	adds[1].Hash = Hash{2}
	adds[2].Hash = Hash{3}
	adds[3].Hash = Hash{4}
	adds[4].Hash = Hash{5}
	adds[5].Hash = Hash{6}
	adds[6].Hash = Hash{7}
	adds[7].Hash = Hash{8}

	// Modify with the additions to simulate txos being added
	_, err := f.Modify(adds, nil)
	if err != nil {
		t.Fatal(err)
	}

	// create blockProof based on the last add in the slice
	blockProof, err := f.ProveBlock(
		[]Hash{adds[lastIdx].Hash})

	if err != nil {
		t.Fatal(err)
	}

	// Confirm that verify block proof works
	shouldBetrue := f.VerifyBlockProof(blockProof)
	if shouldBetrue != true {
		t.Fail()
		t.Logf("Block failed to verify")
	}

	// delete last leaf and add a new leaf
	adds = make([]LeafTXO, 1)
	adds[0].Hash = Hash{9}
	_, err = f.Modify(adds, []uint64{lastIdx})
	if err != nil {
		t.Fatal(err)
	}

	// Attempt to verify block proof with deleted element
	shouldBeFalse := f.VerifyBlockProof(blockProof)
	if shouldBeFalse != false {
		t.Fail()
		t.Logf("Block verified with old proof. Double spending allowed.")
	}
}
