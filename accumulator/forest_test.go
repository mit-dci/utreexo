package accumulator

import (
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"sort"
	"testing"
	"testing/quick"
)

func TestDeleteReverseOrder(t *testing.T) {
	f := NewForest(nil, false, "", 0)
	leaf1 := Leaf{Hash: Hash{1}}
	leaf2 := Leaf{Hash: Hash{2}}
	_, err := f.Modify([]Leaf{leaf1, leaf2}, nil)
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
	numAdds := uint32(10)

	f := NewForest(nil, false, "", 0)

	sc := NewSimChain(0x07)

	for b := 0; b < 1000; b++ {

		adds, _, delHashes := sc.NextBlock(numAdds)

		bp, err := f.ProveBatch(delHashes)
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.Modify(adds, bp.Targets)
		if err != nil {
			t.Fatal(err)
		}

		fmt.Printf("nl %d %s", f.numLeaves, f.ToString())
	}
}

func TestCowForestAddDelComp(t *testing.T) {
	numAdds := uint32(1000)

	tmpDir := os.TempDir()
	cowF := NewForest(nil, false, tmpDir, 2500)
	memF := NewForest(nil, false, "", 0)

	sc := NewSimChain(0x07)
	sc.lookahead = 400

	for b := 0; b <= 1000; b++ {
		adds, _, delHashes := sc.NextBlock(numAdds)

		cowBP, err := cowF.ProveBatch(delHashes)
		if err != nil {
			t.Fatal(err)
		}

		memBP, err := memF.ProveBatch(delHashes)
		if err != nil {
			t.Fatal(err)
		}
		_, err = cowF.Modify(adds, cowBP.Targets)
		if err != nil {
			t.Fatal(err)
		}
		_, err = memF.Modify(adds, memBP.Targets)
		if err != nil {
			t.Fatal(err)
		}
		if b%100 == 0 {
			equal, wrongPos, wrongPosH := checkIfEqual(cowF, memF)
			if !equal {
				cowFile, err := os.OpenFile("cowlog",
					os.O_CREATE|os.O_RDWR, 666)
				if err != nil {
					panic(err)
				}
				cowstring := fmt.Sprintf("nl %d %s\n", cowF.numLeaves, cowF.ToString())
				cowFile.WriteString(cowstring)

				memFile, err := os.OpenFile("memlog",
					os.O_CREATE|os.O_RDWR, 666)
				if err != nil {
					panic(err)
				}

				memstring := fmt.Sprintf("nl %d %s\n", memF.numLeaves, memF.ToString())
				memFile.WriteString(memstring)
				s := fmt.Sprintf("forests are not equal\n")
				s += fmt.Sprintf("forestRows in f: %d\n: ", cowF.rows)
				s += fmt.Sprintf("wrongPos: %x\n", wrongPos)
				s += fmt.Sprintf("wrongPosH %x\n", wrongPosH)
				t.Fatal(s)
			}
		}
	}
	equal, wrongPos, wrongPosH := checkIfEqual(cowF, memF)
	if !equal {
		cowFile, err := os.OpenFile("cowlog",
			os.O_CREATE|os.O_RDWR, 666)
		if err != nil {
			panic(err)
		}
		cowstring := fmt.Sprintf("nl %d %s\n", cowF.numLeaves, cowF.ToString())
		cowFile.WriteString(cowstring)

		memFile, err := os.OpenFile("memlog",
			os.O_CREATE|os.O_RDWR, 666)
		if err != nil {
			panic(err)
		}

		memstring := fmt.Sprintf("nl %d %s\n", memF.numLeaves, memF.ToString())
		memFile.WriteString(memstring)
		s := fmt.Sprintf("forests are not equal\n")
		s += fmt.Sprintf("forestRows in f: %d\n: ", cowF.rows)
		s += fmt.Sprintf("wrongPos: %x\n", wrongPos)
		s += fmt.Sprintf("wrongPosH %x\n", wrongPosH)
		t.Fatal(s)
	}
}

// checkIfEqual checks if the forest differ returns true for equal and if not, returns
// the positions and the hashes
func checkIfEqual(cowF, memF *Forest) (bool, []uint64, []Hash) {
	cowFH := cowF.rows
	memFH := memF.rows

	if cowFH != memFH {
		panic("forestRows don't equal")
	}

	var pos uint8
	for h := uint8(0); h <= memFH; h++ {
		rowlen := uint8(1 << (memFH - h))

		for j := uint8(0); j < rowlen; j++ {
			if cowF.data.size() != memF.data.size() {
				s := fmt.Sprintf("sizes don't equal"+
					"cow: %d, mem: %d\n", cowF.data.size(), memF.data.size())
				panic(s)
			}
			ok := memF.data.size() >= uint64(pos)
			if ok {
				memH := memF.data.read(uint64(pos))
				cowH := cowF.data.read(uint64(pos))
				if memH != cowH {
					s := fmt.Sprintf("hashes aren't equal at gpos: %d "+"mem: %x cow: %x\n", pos, memH, cowH)
					panic(s)
				}
			}
			pos++
		}

	}

	return true, []uint64{}, []Hash{}
}

func TestCowForestAddDel(t *testing.T) {
	numAdds := uint32(10)

	tmpDir := os.TempDir()
	cowF := NewForest(nil, false, tmpDir, 500)

	sc := NewSimChain(0x07)
	sc.lookahead = 400

	for b := 0; b < 1000; b++ {

		adds, _, delHashes := sc.NextBlock(numAdds)

		cowBP, err := cowF.ProveBatch(delHashes)
		if err != nil {
			t.Fatal(err)
		}
		_, err = cowF.Modify(adds, cowBP.Targets)
		if err != nil {
			t.Fatal(err)
		}

		fmt.Printf("nl %d %s\n", cowF.numLeaves, cowF.ToString())
	}
}

func TestForestFixed(t *testing.T) {
	f := NewForest(nil, false, "", 0)
	numadds := 5
	numdels := 3
	adds := make([]Leaf, numadds)
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
	fmt.Print(f.ToString())
	fmt.Print(f.PrintPositionMap())
	_, err = f.Modify(nil, dels)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Print(f.ToString())
	fmt.Print(f.PrintPositionMap())
}

// Add 2. delete 1.  Repeat.
func Test2Fwd1Back(t *testing.T) {
	f := NewForest(nil, false, "", 0)
	var absidx uint32
	adds := make([]Leaf, 2)

	for i := 0; i < 100; i++ {

		for j, _ := range adds {
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
		fmt.Printf("\t\t\t########### block %d ##########\n\n", i)

		// add 2
		_, err := f.Modify(adds, nil)
		if err != nil {
			t.Fatal(err)
		}

		s := f.ToString()
		fmt.Printf(s)

		// get proof for the first
		_, err = f.Prove(adds[0].Hash)
		if err != nil {
			t.Fatal(err)
		}

		// delete the first
		//		err = f.Modify(nil, []util.Hash{p.Payload})
		//		if err != nil {
		//			t.Fatal(err)
		//		}

		//		s = f.ToString()
		//		fmt.Printf(s)

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
func TestAddxDelyLeftFullBatchProof(t *testing.T) {
	for x := 0; x < 10; x++ {
		for y := 0; y < x; y++ {
			err := addDelFullBatchProof(x, y)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

}

// Add x, delete y, construct & reconstruct blockproof
func addDelFullBatchProof(nAdds, nDels int) error {
	if nDels > nAdds-1 {
		return fmt.Errorf("too many deletes")
	}

	f := NewForest(nil, false, "", 0)
	adds := make([]Leaf, nAdds)

	for j, _ := range adds {
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
	bp, err := f.ProveBatch(addHashes[:nDels])
	if err != nil {
		return err
	}
	// check block proof.  Note this doesn't delete anything, just proves inclusion
	worked, _, _ := verifyBatchProof(bp, f.getRoots(), f.numLeaves, nil)
	//	worked := f.VerifyBatchProof(bp)

	if !worked {
		return fmt.Errorf("VerifyBatchProof failed")
	}
	fmt.Printf("VerifyBatchProof worked\n")
	return nil
}

func TestDeleteNonExisting(t *testing.T) {
	f := NewForest(nil, false, "", 0)
	deletions := []uint64{0}
	_, err := f.Modify(nil, deletions)
	if err == nil {
		t.Fatal(fmt.Errorf(
			"shouldn't be able to delete non-existing leaf 0 from empty forest"))
	}
}

func TestSmallRandomForests(t *testing.T) {
	rand := rand.New(rand.NewSource(0))

	for i := 0; i < 1000; i++ {
		// The forest instance to test in this iteration of the loop
		f := NewForest(nil, false, "", 0)

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
		var chosenUndeletedLeaf Leaf
		atLeastOneLeafRemains := false

		// forest.Modify takes a slice, so we'll copy
		// adds into this slice:
		addsSlice := make([]Leaf, len(adds))

		// This stores the leaves that are to be deleted.
		// We need to store the LeafTXO's or we won't be able
		// to find the positions after inserting all items.
		leavesToDeleteSet := make(map[Leaf]struct{})

		i := 0
		for firstLeafHashByte, deleteLater := range adds {
			// We put a one in the hash too, so that we won't
			// generate an all-zero hash, which is not allowed.
			leafTxo := Leaf{Hash: Hash{firstLeafHashByte, 1}}
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
		deletions := make([]int, len(leavesToDeleteSet))
		i = 0
		for leafTxo, _ := range leavesToDeleteSet {
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
			blockProof, err := f.ProveBatch(
				[]Hash{
					chosenUndeletedLeaf.Hash,
				})
			if err != nil {
				t.Fatalf("proveblock failed proving existing leaf: %v", err)
			}

			if !(f.VerifyBatchProof(blockProof)) {
				t.Fatal("verifyblockproof failed verifying proof for existing leaf")
			}
		}
	}
}
