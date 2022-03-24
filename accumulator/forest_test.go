package accumulator

import (
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"sort"
	"testing"
	"testing/quick"
)

func TestForestSwaplessSimple(t *testing.T) {
	var tests = []struct {
		startLeaves []Leaf
		dels        []uint64
		expected    map[uint64]Hash
	}{
		{
			[]Leaf{
				{Hash: Hash{1}},
				{Hash: Hash{2}},
				{Hash: Hash{3}},
				{Hash: Hash{4}},
				{Hash: Hash{5}},
				{Hash: Hash{6}},
				{Hash: Hash{7}},
				{Hash: Hash{8}},
			},
			[]uint64{0, 1, 2, 3, 4, 6, 7},
			map[uint64]Hash{
				0:  empty,
				1:  empty,
				2:  empty,
				3:  empty,
				4:  empty,
				5:  empty,
				6:  empty,
				7:  empty,
				8:  empty,
				9:  empty,
				10: empty,
				11: empty,
				12: empty,
				13: empty,
				14: Hash{6},
			},
		},

		// 14
		// |---------------\
		// 12              13
		// |-------\       |-------\
		// 08      09      10      11
		// |---\   |---\   |---\   |---\
		// 00* 01* 02* 03* 04  05  06  07
		{
			[]Leaf{
				{Hash: Hash{1}},
				{Hash: Hash{2}},
				{Hash: Hash{3}},
				{Hash: Hash{4}},
				{Hash: Hash{5}},
				{Hash: Hash{6}},
				{Hash: Hash{7}},
				{Hash: Hash{8}},
			},
			[]uint64{0, 1, 2, 3},
			map[uint64]Hash{
				0:  empty,
				1:  empty,
				2:  empty,
				3:  empty,
				4:  empty,
				5:  empty,
				6:  empty,
				7:  empty,
				8:  Hash{5},
				9:  Hash{6},
				10: Hash{7},
				11: Hash{8},
				12: parentHash(Hash{5}, Hash{6}),
				13: parentHash(Hash{7}, Hash{8}),
				14: parentHash(
					parentHash(Hash{5}, Hash{6}),
					parentHash(Hash{7}, Hash{8})),
			},
		},

		// 14
		// |---------------\
		// 12              13
		// |-------\       |-------\
		// 08      09      10      11
		// |---\   |---\   |---\   |---\
		// 00* 01* 02  03* 04  05  06  07
		{
			[]Leaf{
				{Hash: Hash{1}},
				{Hash: Hash{2}},
				{Hash: Hash{3}},
				{Hash: Hash{4}},
				{Hash: Hash{5}},
				{Hash: Hash{6}},
				{Hash: Hash{7}},
				{Hash: Hash{8}},
			},
			[]uint64{0, 1, 3},
			map[uint64]Hash{
				0:  empty,
				1:  empty,
				2:  empty,
				3:  empty,
				4:  Hash{5},
				5:  Hash{6},
				6:  Hash{7},
				7:  Hash{8},
				8:  empty,
				9:  empty,
				10: parentHash(Hash{5}, Hash{6}),
				11: parentHash(Hash{7}, Hash{8}),
				12: Hash{3},
				13: parentHash(
					parentHash(Hash{5}, Hash{6}),
					parentHash(Hash{7}, Hash{8})),
				14: parentHash(
					Hash{3},
					parentHash(parentHash(Hash{5}, Hash{6}),
						parentHash(Hash{7}, Hash{8})),
				),
			},
		},

		{
			[]Leaf{
				{Hash: Hash{1}},
				{Hash: Hash{2}},
				{Hash: Hash{3}},
				{Hash: Hash{4}},
				{Hash: Hash{5}},
				{Hash: Hash{6}},
				{Hash: Hash{7}},
				{Hash: Hash{8}},
				{Hash: Hash{9}},
			},
			[]uint64{0, 1, 2},
			map[uint64]Hash{
				0:  empty,
				1:  empty,
				2:  empty,
				3:  empty,
				4:  Hash{5},
				5:  Hash{6},
				6:  Hash{7},
				7:  Hash{8},
				8:  Hash{9},
				16: empty,
				17: empty,
				18: parentHash(Hash{5}, Hash{6}),
				19: parentHash(Hash{7}, Hash{8}),
				25: parentHash(
					parentHash(Hash{5}, Hash{6}),
					parentHash(Hash{7}, Hash{8})),
				28: parentHash(
					Hash{4},
					parentHash(
						parentHash(Hash{5}, Hash{6}),
						parentHash(Hash{7}, Hash{8})),
				),
			},
		},
	}

	for i, test := range tests {
		//if i != 1 {
		//	continue
		//}

		f := NewForest(RamForest, nil, "", 0)

		_, err := f.Modify(test.startLeaves, nil)
		if err != nil {
			t.Fatal(err)
		}

		fmt.Println(f.ToString())

		err = f.removeSwapless(test.dels)
		if err != nil {
			t.Fatal(err)
		}

		fmt.Println(f.ToString())

		for pos, expectedHash := range test.expected {
			hash := f.data.read(pos)

			if hash != expectedHash {
				t.Errorf("TestForestSwaplessSimple Fail: test %d failed "+
					"for position %d, expected %s got %s",
					i, pos, hex.EncodeToString(expectedHash[:]),
					hex.EncodeToString(hash[:]))
			}
		}
	}
}

func TestSwapLessAdd(t *testing.T) {
	f := NewForest(RamForest, nil, "", 0)
	leaves := []Leaf{
		{Hash: Hash{1}},
		{Hash: Hash{2}},
		{Hash: Hash{3}},
		{Hash: Hash{4}},
		{Hash: Hash{5}},
		{Hash: Hash{6}},
		{Hash: Hash{7}},
		{Hash: Hash{8}},
		{Hash: Hash{9}},
	}

	// remap to expand the forest if needed
	numdels, numadds := 0, len(leaves)
	delta := int64(numadds - numdels) // watch 32/64 bit
	for int64(f.numLeaves)+delta > int64(1<<f.rows) {
		// 1<<f.rows, f.numLeaves+delta)
		err := f.reMap(f.rows + 1)
		if err != nil {
			t.Fatal(err)
		}
	}
	f.addSwapless(leaves)

	fmt.Println(f.ToString())
	fmt.Println(f.positionMap)

	dels := []uint64{0, 1, 2}
	f.removeSwapless(dels)

	fmt.Println(f.ToString())
	fmt.Println(f.positionMap)

	f.addSwapless([]Leaf{{Hash: Hash{10}}})

	fmt.Println(f.ToString())
	fmt.Println(f.positionMap)

	f.removeSwapless([]uint64{8, 9})
	fmt.Println(f.ToString())
	fmt.Println(f.positionMap)

	adds := []Leaf{
		{Hash: Hash{11}},
	}

	f.addSwapless(adds)
	fmt.Println(f.ToString())
	fmt.Println(f.positionMap)

	adds = []Leaf{
		{Hash: Hash{12}},
	}

	f.addSwapless(adds)
	fmt.Println(f.ToString())
	fmt.Println(f.positionMap)

	adds = []Leaf{
		{Hash: Hash{13}},
		{Hash: Hash{14}},
		{Hash: Hash{15}},
		{Hash: Hash{16}},
	}
	f.addSwapless(adds)
	fmt.Println(f.ToString())
	fmt.Println(f.positionMap)

	f.removeSwapless([]uint64{12})
	fmt.Println(f.ToString())
	fmt.Println(f.positionMap)

	proofPositions := NewPositionList()
	defer proofPositions.Free()

	targetss := []uint64{4, 20, 24}
	fmt.Printf("targets %v, proofpos %v\n", targetss, proofPositions.list)
	//ProofPositionsSwapless(targetss, f.numLeaves, f.rows, &proofPositions.list)
	ProofPositions(targetss, f.numLeaves, f.rows, &proofPositions.list)

	fmt.Printf("targets %v, proofpos %v\n", targetss, proofPositions.list)
	fmt.Printf("numLeaves: %d, f.rows: %d\n", f.numLeaves, f.rows)
}

func TestDeleteReverseOrder(t *testing.T) {
	f := NewForest(RamForest, nil, "", 0)
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

func TestExtractRow(t *testing.T) {
	targets := []uint64{4, 20, 24}
	forestRow := uint8(4)

	//fmt.Println(extractRow(targets, forestRow))

	fmt.Println("targets", targets)
	fmt.Println(extractRow(targets, forestRow, 2))
	fmt.Println("targets", targets)
	//fmt.Println(extractRow(targets, forestRow, 1))
	//fmt.Println(extractRow(targets, forestRow, 2))
}

func TestForestAddDel(t *testing.T) {
	numAdds := uint32(10)

	f := NewForest(RamForest, nil, "", 0)

	sc := newSimChain(0x07)

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
	// Function for writing logs.
	writeLog := func(cowF, memF *Forest) {
		cowstring := fmt.Sprintf("cowForest: nl %d %s\n",
			cowF.numLeaves, cowF.ToString())
		fmt.Println(cowstring)

		memstring := fmt.Sprintf("memForest: nl %d %s\n",
			memF.numLeaves, memF.ToString())
		fmt.Println(memstring)
	}

	tmpDir := os.TempDir()
	defer os.RemoveAll(tmpDir)

	cowF := NewForest(CowForest, nil, tmpDir, 2500)
	memF := NewForest(RamForest, nil, "", 0)
	numAdds := uint32(1000)

	sc := newSimChain(0x07)
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
			err := cowF.AssertEqual(memF)
			if err != nil {
				writeLog(cowF, memF)
				t.Fatal(err)
			}
		}
	}

	err := cowF.AssertEqual(memF)
	if err != nil {
		writeLog(cowF, memF)
		t.Fatal(err)
	}
}

func TestCowForestAddDel(t *testing.T) {
	numAdds := uint32(10)

	tmpDir := os.TempDir()
	cowF := NewForest(CowForest, nil, tmpDir, 500)

	sc := newSimChain(0x07)
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
	f := NewForest(RamForest, nil, "", 0)
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
	f := NewForest(RamForest, nil, "", 0)
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

	f := NewForest(RamForest, nil, "", 0)
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

	leavesToProve := addHashes[:nDels]

	// get block proof
	bp, err := f.ProveBatch(leavesToProve)
	if err != nil {
		return err
	}
	// check block proof.  Note this doesn't delete anything, just proves inclusion
	_, _, err = verifyBatchProof(leavesToProve, bp, f.GetRoots(), f.numLeaves, nil)
	if err != nil {
		return fmt.Errorf("VerifyBatchProof failed. Error: %s", err.Error())
	}
	fmt.Printf("VerifyBatchProof worked\n")
	return nil
}

func TestDeleteNonExisting(t *testing.T) {
	f := NewForest(RamForest, nil, "", 0)
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
		f := NewForest(RamForest, nil, "", 0)

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

			err = f.VerifyBatchProof([]Hash{chosenUndeletedLeaf.Hash}, blockProof)
			if err != nil {
				retErr := fmt.Errorf("verifyblockproof failed verifying proof for existing leaf."+
					" Error: %s", err.Error())
				t.Fatal(retErr)
			}
		}
	}
}
