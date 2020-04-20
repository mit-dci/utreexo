package utreexo

import (
	"fmt"
	"log"
	"math/rand"
	"reflect"
	"sort"
	"testing"
	"testing/quick"
)

func TestDeleteReverseOrder(t *testing.T) {
	f := NewForest(nil)
	leaf1 := LeafTXO{Hash: Hash{1}}
	leaf2 := LeafTXO{Hash: Hash{2}}
	_, err := f.Modify([]LeafTXO{leaf1, leaf2}, nil)
	if err != nil {
		t.Fail()
	}
	_, err = f.Modify(nil, []uint64{0, 1})
	if err != nil {
		t.Log(err)
		t.Fatal("could not delete leaves 1 and 0")
	}
}

func TestForestAddDel(t *testing.T) {
	logger := NewLogger(t)

	numAdds := uint32(10)

	f := NewForest(nil)

	sc := NewSimChain(0x07)
	sc.lookahead = 400

	for b := 0; b < 1000; b++ {

		adds, delHashes := sc.NextBlock(numAdds)

		bp, err := f.ProveBlock(delHashes)
		if err != nil {
			t.Fatal(err)
		}

		_, err = f.Modify(adds, bp.Targets)
		if err != nil {
			t.Fatal(err)
		}

		logger.Printf("nl %d %s", f.numLeaves, f.ToString())
	}
}

func TestForestFixed(t *testing.T) {
	logger := NewLogger(t)
	f := NewForest(nil)
	numadds := 5
	numdels := 3
	adds := make([]LeafTXO, numadds)
	dels := make([]uint64, numdels)

	for j, _ := range adds {
		adds[j].Hash[0] = uint8(j >> 8)
		adds[j].Hash[1] = uint8(j)
		adds[j].Hash[3] = 0xaa
	}
	for k, _ := range dels {
		dels[k] = uint64(k)
	}

	_, err := f.Modify(adds, nil)
	if err != nil {
		t.Fatal(err)
	}
	logger.Printf(f.ToString())
	logger.Printf(f.PrintPositionMap())
	_, err = f.Modify(nil, dels)
	if err != nil {
		t.Fatal(err)
	}
	logger.Printf(f.ToString())
	logger.Printf(f.PrintPositionMap())
}

// Add 2. delete 1.  Repeat.
func Test2Fwd1Back(t *testing.T) {
	logger := NewLogger(t)
	f := NewForest(nil)
	var absidx uint32
	adds := make([]LeafTXO, 2)

	for i := 0; i < 100; i++ {

		for j := range adds {
			adds[j].Hash[0] = uint8(absidx>>8) | 0xa0
			adds[j].Hash[1] = uint8(absidx)
			adds[j].Hash[3] = 0xaa
			absidx++
			//		if i%30 == 0 {
			//			utree.Track(adds[i])
			//			trax = append(trax, adds[i])
			//		}
		}

		//		t.Logf("-------- block %d\n", i)
		logger.Printf("\t\t\t########### block %d ##########\n\n", i)

		// add 2
		_, err := f.Modify(adds, nil)
		if err != nil {
			t.Fatal(err)
		}

		s := f.ToString()
		logger.Printf(s)

		// get proof for the first
		_, err = f.Prove(adds[0].Hash)
		if err != nil {
			t.Fatal(err)
		}

		// delete the first
		//		err = f.Modify(nil, []Hash{p.Payload})
		//		if err != nil {
		//			t.Fatal(err)
		//		}

		//		s = f.ToString()
		//		logger.Printf(s)

		// get proof for the 2nd
		keep, err := f.Prove(adds[1].Hash)
		if err != nil {
			t.Fatal(err)
		}
		// check proof

		worked := f.Verify(keep)
		if !worked {
			t.Fatalf("proof at position %d, length %d failed to verify\n",
				keep.Position, len(keep.Siblings))
		}
	}
}

// Add and delete variable numbers, repeat.
// deletions are all on the left side and contiguous.
func TestAddxDelyLeftFullBlockProof(t *testing.T) {
	logger := NewLogger(t)
	for x := 0; x < 10; x++ {
		for y := 0; y < x; y++ {
			err := AddDelFullBlockProof(logger, x, y)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

}

// Add x, delete y, construct & reconstruct blockproof
func AddDelFullBlockProof(logger *log.Logger, nAdds, nDels int) error {
	if nDels > nAdds-1 {
		return fmt.Errorf("too many deletes")
	}

	f := NewForest(nil)
	adds := make([]LeafTXO, nAdds)

	for j := range adds {
		adds[j].Hash[0] = uint8(j>>8) | 0xa0
		adds[j].Hash[1] = uint8(j)
		adds[j].Hash[3] = 0xaa
	}

	// add x
	_, err := f.Modify(adds, nil)
	if err != nil {
		return err
	}
	addHashes := make([]Hash, len(adds))
	for i, h := range adds {
		addHashes[i] = h.Hash
	}

	// get block proof
	bp, err := f.ProveBlock(addHashes[:nDels])
	if err != nil {
		return err
	}

	// check block proof.  Note this doesn't delete anything, just proves inclusion
	worked, _ := VerifyBlockProof(bp, f.GetTops(), f.numLeaves, f.height)
	//	worked := f.VerifyBlockProof(bp)

	if !worked {
		return fmt.Errorf("VerifyBlockProof failed")
	}
	logger.Printf("VerifyBlockProof worked\n")
	return nil
}

func TestDeleteNonExisting(t *testing.T) {
	f := NewForest(nil)
	deletions := []uint64{0}
	_, err := f.Modify(nil, deletions)
	if err == nil {
		t.Fatalf(
			"shouldn't be able to delete non-existing leaf 0 from empty forest")
	}
}

func TestSmallRandomForests(t *testing.T) {
	rand := rand.New(rand.NewSource(0))

	for i := 0; i < 1000; i++ {
		// The forest instance to test in this iteration of the loop
		f := NewForest(nil)

		// We use 'quick' to generate testing data:
		// we interpret the keys as leaf hashes and the values
		// as indicating whether we should delete the leaf on
		// our second call to Modify
		value, ok := quick.Value(reflect.TypeOf((map[uint8]bool)(nil)), rand)
		if !ok {
			t.Fatal("could not create uint8->bool map")
		}
		adds := value.Interface().(map[uint8]bool)

		// This is the leaf that we will test proofs for
		// if we happen to generate testing data that
		// doesn't leave an empty tree.
		var chosenUndeletedLeaf LeafTXO
		atLeastOneLeafRemains := false

		// forest.Modify takes a slice, so we'll copy
		// adds into this slice:
		addsSlice := make([]LeafTXO, len(adds))

		// This stores the leaves that are to be deleted.
		// We need to store the LeafTXO's or we won't be able
		// to find the positions after inserting all items.
		leavesToDeleteSet := make(map[LeafTXO]struct{})

		i := 0
		for firstLeafHashByte, deleteLater := range adds {
			// We put a one in the hash too, so that we won't
			// generate an all-zero hash, which is not allowed.
			leafTxo := LeafTXO{Hash: Hash{firstLeafHashByte, 1}}
			addsSlice[i] = leafTxo
			if deleteLater {
				leavesToDeleteSet[leafTxo] = struct{}{}
			} else {
				atLeastOneLeafRemains = true
				chosenUndeletedLeaf = leafTxo
			}
			i++
		}

		_, err := f.Modify(addsSlice, nil)
		if err != nil {
			t.Fatalf("could not add leafs to empty forest: %v", err)
		}

		// We convert leavesToDeleteSet to an array, so we
		// can sort it.
		// Modify requires a sorted list of leaves to delete.
		// We use int because we can't sort uint64's.
		var deletions []int
		deletions = make([]int, len(leavesToDeleteSet))
		i = 0
		for leafTxo := range leavesToDeleteSet {
			deletions[i] = int(f.positionMap[leafTxo.Mini()])
			i++
		}
		sort.Ints(deletions)

		// We convert to uint64 so that we can call Modify
		deletions_uint64 := make([]uint64, len(deletions))
		i = 0
		for _, el := range deletions {
				deletions_uint64[i] = uint64(el)
				i++
		}
		t.Logf("\nadding (the bool values are whether deletion happens):\n%v\ndeleting:\n%v\n", adds, deletions_uint64)

		_, err = f.Modify(nil, deletions_uint64)
		if err != nil {
			t.Fatalf("could not delete leaves from filled forest: %v", err)
		}

		// If the tree we filled isn't empty, and contains a node we didn't delete,
		// we should be able to make a proof for that leaf
		if atLeastOneLeafRemains {
			blockProof, err := f.ProveBlock(
				[]Hash {
					chosenUndeletedLeaf.Hash,
				})
			if err != nil {
				t.Fatalf("proveblock failed proving existing leaf: %v", err)
			}

			if !(f.VerifyBlockProof(blockProof)) {
				t.Fatal("verifyblockproof failed verifying proof for existing leaf")
			}
		}
	}
}
