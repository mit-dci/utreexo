package utreexo

import (
	"fmt"
	"testing"
)

func TestForestAddDel(t *testing.T) {

	numAdds := 10
	numDels := 5

	for b := 0; b < 1000; b++ {
		//  these should stay the same
		f := NewForest() // bottom up modified forest

		delMap := make(map[uint64]bool)
		adds := make([]LeafTXO, numAdds)
		for j := range adds {
			adds[j].Hash[1] = uint8(j)
			adds[j].Hash[3] = 0xcc
			delMap[uint64(j)] = true
		}

		_, err := f.Modify(adds, nil)
		if err != nil {
			t.Fatal(err)
		}

		var k int
		dels := make([]uint64, numDels)
		for i, _ := range delMap {
			dels[k] = i
			k++
			if k >= numDels {
				break
			}
		}

		err = f.removev3(dels) // was v2
		if err != nil {
			t.Fatal(err)
		}

		fmt.Printf("nl %d %s", f.numLeaves, f.ToString())
	}
}

// Add 2. delete 1.  Repeat.
func Test2Fwd1Back(t *testing.T) {
	f := NewForest()
	var absidx uint32
	adds := make([]LeafTXO, 2)

	for i := 0; i < 1000; i++ {

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
		//		err = f.Modify(nil, []Hash{p.Payload})
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
			t.Fatalf("proof at postition %d, length %d failed to verify\n",
				keep.Position, len(keep.Siblings))
		}
	}
}

// Add and delete variable numbers, repeat.
// deletions are all on the left side and contiguous.
func TestAddxDelyLeftFullBlockProof(t *testing.T) {
	for x := 0; x < 10; x++ {
		for y := 0; y < x; y++ {
			err := AddDelFullBlockProof(x, y)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

}

// Add x, delete y, construct & reconstruct blockproof
func AddDelFullBlockProof(nAdds, nDels int) error {
	if nDels > nAdds-1 {
		return fmt.Errorf("too many deletes")
	}

	f := NewForest()
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
	fmt.Printf("VerifyBlockProof worked\n")
	return nil
}
