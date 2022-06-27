package accumulator

import (
	"fmt"
	"math/rand"
	"testing"
)

func TestPollardFixed(t *testing.T) {
	rand.Seed(2)
	//	err := pollardMiscTest()
	//	if err != nil {
	//		t.Fatal(err)
	//	}
	//	for i := 6; i < 100; i++ {
	err := fixedPollard(7)
	if err != nil {
		t.Fatal(err)
	}
}
func TestPollardSimpleIngest(t *testing.T) {
	f := NewForest(RamForest, nil, "", 0)
	adds := make([]Leaf, 15)
	for i := 0; i < len(adds); i++ {
		adds[i].Hash[0] = uint8(i + 1)
	}

	f.Modify(adds, []uint64{})
	fmt.Println(f.ToString())

	hashes := make([]Hash, len(adds))
	for i := 0; i < len(hashes); i++ {
		hashes[i] = adds[i].Hash
	}

	bp, _ := f.ProveBatch(hashes)

	var p Pollard
	p.Modify(adds, nil)
	// Modify the proof so that the verification should fail.
	if len(bp.Proof) <= 0 {
		bp.Proof = make([]Hash, 1)
		bp.Proof[0][0] = 0xFF
	}
	err := p.IngestBatchProof(hashes, bp)
	if err == nil {
		t.Fatal("BatchProof valid after modification. Accumulator validation failing")
	}
}

// fixedPollard adds and removes things in a non-random way
func fixedPollard(leaves int32) error {
	fmt.Printf("\t\tpollard test add %d remove 1\n", leaves)
	f := NewForest(RamForest, nil, "", 0)

	leafCounter := uint64(0)

	dels := []uint64{2, 5, 6}

	// they're all forgettable
	adds := make([]Leaf, leaves)

	// make a bunch of unique adds & make an expiry time and add em to
	// the TTL map
	for j, _ := range adds {
		adds[j].Hash[1] = uint8(leafCounter)
		adds[j].Hash[2] = uint8(leafCounter >> 8)
		adds[j].Hash[3] = uint8(leafCounter >> 16)
		adds[j].Hash[4] = uint8(leafCounter >> 24)
		adds[j].Hash[9] = uint8(0xff)

		// the first utxo added lives forever.
		// (prevents leaves from going to 0 which is buggy)
		adds[j].Remember = true
		leafCounter++
	}

	// apply adds and deletes to the bridge node (could do this whenever)
	_, err := f.Modify(adds, nil)
	if err != nil {
		return err
	}
	fmt.Printf("forest  post del %s", f.ToString())

	var p Pollard

	err = p.add(adds)
	if err != nil {
		return err
	}

	fmt.Printf("pollard post add %s", p.ToString())

	// err = p.rem2(dels)
	// if err != nil {
	// return err
	// }

	_, err = f.Modify(nil, dels)
	if err != nil {
		return err
	}
	fmt.Printf("forest  post del %s", f.ToString())

	fmt.Printf("pollard post del %s", p.ToString())

	if !p.equalToForest(f) {
		return fmt.Errorf("p != f (leaves)")
	}

	return nil
}

func TestCache(t *testing.T) {
	// simulate blocks with simchain
	chain := newSimChain(7)
	chain.lookahead = 8

	f := NewForest(RamForest, nil, "", 0)
	var p Pollard

	// this leaf map holds all the leaves at the current height and is used to check if the pollard
	// is caching leaf proofs correctly
	leaves := make(map[Hash]Leaf)

	for i := 0; i < 16; i++ {
		adds, _, delHashes := chain.NextBlock(8)
		proof, err := f.ProveBatch(delHashes)
		if err != nil {
			t.Fatal("ProveBatch failed", err)
		}

		_, err = f.Modify(adds, proof.Targets)
		if err != nil {
			t.Fatal("Modify failed", err)
		}

		err = p.IngestBatchProof(delHashes, proof)
		if err != nil {
			t.Fatal("IngestBatchProof failed", err)
		}

		err = p.Modify(adds, proof.Targets)
		if err != nil {
			t.Fatal("Modify failed", err)
		}

		// remove deleted leaves from the leaf map
		for _, del := range delHashes {
			fmt.Printf("del %x\n", del.Mini())
			delete(leaves, del)
		}
		// add new leaves to the leaf map
		for _, leaf := range adds {
			fmt.Printf("add %x rem:%v\n", leaf.Hash.Mini(), leaf.Remember)
			leaves[leaf.Hash] = leaf
		}

		for hash, l := range leaves {
			leafProof, err := f.ProveBatch([]Hash{hash})
			if err != nil {
				t.Fatal("Prove failed", err)
			}
			pos := leafProof.Targets[0]

			n, nsib, _, err := p.readPos(pos)
			if err != nil {
				t.Fatal("could not read leaf pos at", pos)
			}

			if pos == p.numLeaves-1 {
				// roots are always cached
				continue
			}

			// If the leaf wasn't marked to be remembered, check if the sibling is remembered.
			// If the sibling is supposed to be remembered, it's ok to remember this leaf as it
			// is the proof for the sibling.
			if !l.Remember && n != nil {
				// If the sibling exists, check if the sibling leaf was supposed to be remembered.
				if nsib != nil {
					sibling := leaves[nsib.data]
					if !sibling.Remember || nsib.data == empty {
						fmt.Println(p.ToString())

						err := fmt.Errorf("leaf at position %d exists but it was added with "+
							"remember=%v and its sibilng with remember=%v. "+
							"polnode remember=%v, polnode sibling remember=%v",
							pos, l.Remember, sibling.Remember, n.remember, nsib.remember)
						t.Fatal(err)
					}
				} else {
					// If the sibling does not exist, fail as this leaf should not be
					// remembered.
					fmt.Println(p.ToString())

					err := fmt.Errorf("leaf at position %d exists but it was added with "+
						"remember=%v and its sibilng is nil. "+
						"polnode remember=%v",
						pos, l.Remember, n.remember)
					t.Fatal(err)
				}
			}

			siblingDoesNotExist := nsib == nil || nsib.data == empty
			if l.Remember && siblingDoesNotExist {
				// the proof for l is not cached even though it should have been because it
				// was added with remember=true.
				fmt.Println(p.ToString())
				err := fmt.Errorf("leaf at position %d exists but it was added with "+
					"remember=%v and its sibilng does not exist. "+
					"polnode remember=%v",
					pos, l.Remember, n.remember)
				t.Fatal(err)
			} else if !l.Remember && !siblingDoesNotExist {
				sibling := leaves[nsib.data]

				// If the sibling exists but it wasn't supposed to be remembered, something's wrong.
				if !sibling.Remember {
					// the proof for l was cached even though it should not have been because it
					// was added with remember = false.
					fmt.Println(p.ToString())

					err := fmt.Errorf("leaf at position %d exists but it was added with "+
						"remember=%v and its sibilng with remember=%v. "+
						"polnode remember=%v, polnode sibling remember=%v",
						pos, l.Remember, sibling.Remember, n.remember, nsib.remember)
					t.Fatal(err)
				}
			}
		}
		fmt.Println(p.ToString())
	}
}
