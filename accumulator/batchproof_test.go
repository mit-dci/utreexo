package accumulator

import (
	"fmt"
	"testing"
)

// TestIncompleteBatchProof tests that a incomplete (missing some hashes) batchproof does not pass verification.
func TestIncompleteBatchProof(t *testing.T) {
	// Create forest in memory
	f := NewForest(nil, false, "", 0)

	// last index to be deleted. Same as blockDels
	lastIdx := uint64(7)

	// Generate adds
	adds := make([]Leaf, 8)
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

	leavesToProve := []Hash{adds[lastIdx].Hash}

	// create blockProof based on the last add in the slice
	blockProof, err := f.ProveBatch(leavesToProve)

	if err != nil {
		t.Fatal(err)
	}

	blockProof.Proof = blockProof.Proof[:len(blockProof.Proof)-1]
	shouldBeFalse := f.VerifyBatchProof(leavesToProve, blockProof)
	if shouldBeFalse != false {
		t.Fail()
		t.Logf("Incomplete proof passes verification")
	}
}

// TestVerifyBlockProof tests that the computedTop is compared to the top in the
// Utreexo forest.
func TestVerifyBatchProof(t *testing.T) {
	// Create forest in memory
	f := NewForest(nil, false, "", 0)

	// last index to be deleted. Same as blockDels
	lastIdx := uint64(7)

	// Generate adds
	adds := make([]Leaf, 8)
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

	leavesToProve := []Hash{adds[lastIdx].Hash}

	// create blockProof based on the last add in the slice
	blockProof, err := f.ProveBatch(leavesToProve)

	if err != nil {
		t.Fatal(err)
	}

	// Confirm that verify block proof works
	shouldBetrue := f.VerifyBatchProof(leavesToProve, blockProof)
	if shouldBetrue != true {
		t.Fail()
		t.Logf("Block failed to verify")
	}

	// delete last leaf and add a new leaf
	adds = make([]Leaf, 1)
	adds[0].Hash = Hash{9}
	_, err = f.Modify(adds, []uint64{lastIdx})
	if err != nil {
		t.Fatal(err)
	}

	// Attempt to verify block proof with deleted element
	shouldBeFalse := f.VerifyBatchProof(leavesToProve, blockProof)
	if shouldBeFalse != false {
		t.Fail()
		t.Logf("Block verified with old proof. Double spending allowed.")
	}
}

// In a two leaf tree:
// We prove one node, then delete the other one.
// Now, the proof of the first node should not pass verification.

// Full explanation: https://github.com/mit-dci/utreexo/pull/95#issuecomment-599390850
func TestProofShouldNotValidateAfterNodeDeleted(t *testing.T) {
	adds := make([]Leaf, 2)
	proofIndex := 1
	adds[0].Hash = Hash{1} // will be deleted
	adds[1].Hash = Hash{2} // will be proven

	f := NewForest(nil, false, "", 0)
	_, err := f.Modify(adds, nil)
	if err != nil {
		t.Fatal(fmt.Errorf("Modify with initial adds: %v", err))
	}

	batchProof, err := f.ProveBatch(
		[]Hash{
			adds[proofIndex].Hash,
		})
	if err != nil {
		t.Fatal(fmt.Errorf("ProveBlock of existing values: %v", err))
	}

	if !f.VerifyBatchProof([]Hash{adds[proofIndex].Hash}, batchProof) {
		t.Fatal(
			fmt.Errorf(
				"proof of %d didn't verify (before deletion)",
				proofIndex))
	}

	_, err = f.Modify(nil, []uint64{0})
	if err != nil {
		t.Fatal(fmt.Errorf("Modify with deletions: %v", err))
	}

	if f.VerifyBatchProof([]Hash{adds[proofIndex].Hash}, batchProof) {
		t.Fatal(
			fmt.Errorf(
				"proof of %d is still valid (after deletion)",
				proofIndex))
	}
}
