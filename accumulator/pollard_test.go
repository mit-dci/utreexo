package accumulator

import (
	"fmt"
	"math/rand"
	"testing"
)

func TestPollardRand(t *testing.T) {
	for z := 0; z < 30; z++ {
		// z := 11221
		// z := 55
		rand.Seed(int64(z))
		fmt.Printf("randseed %d\n", z)
		err := pollardRandomRemember(20)
		if err != nil {
			fmt.Printf("randseed %d\n", z)
			t.Fatal(err)
		}
	}
}

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
	f := NewForest(nil, false, "", 0)
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

func pollardRandomRemember(blocks int32) error {
	f := NewForest(nil, false, "", 0)

	var p Pollard

	sn := newSimChain(0x07)
	sn.lookahead = 400
	for b := int32(0); b < blocks; b++ {
		adds, _, delHashes := sn.NextBlock(rand.Uint32() & 0xff)

		fmt.Printf("\t\t\tstart block %d del %d add %d - %s\n",
			sn.blockHeight, len(delHashes), len(adds), p.Stats())

		// get proof for these deletions (with respect to prev block)
		bp, err := f.ProveBatch(delHashes)
		if err != nil {
			return err
		}

		// verify proofs on rad node
		err = p.IngestBatchProof(delHashes, bp)
		if err != nil {
			return err
		}

		// apply adds and deletes to the bridge node (could do this whenever)
		_, err = f.Modify(adds, bp.Targets)
		if err != nil {
			return err
		}
		// TODO fix: there is a leak in forest.Modify where sometimes
		// the position map doesn't clear out and a hash that doesn't exist
		// any more will be stuck in the positionMap.  Wastes a bit of memory
		// and seems to happen when there are moves to and from a location
		// Should fix but can leave it for now.

		err = f.sanity()
		if err != nil {
			fmt.Printf("frs broke %s", f.ToString())
			for h, p := range f.positionMap {
				fmt.Printf("%x@%d ", h[:4], p)
			}
			return err
		}
		err = f.PosMapSanity()
		if err != nil {
			fmt.Print(f.ToString())
			return err
		}

		// apply adds / dels to pollard
		err = p.Modify(adds, bp.Targets)
		if err != nil {
			return err
		}

		fmt.Printf("pol postadd %s", p.ToString())

		fmt.Printf("frs postadd %s", f.ToString())

		// check all leaves match
		if !p.equalToForestIfThere(f) {
			return fmt.Errorf("pollard and forest leaves differ")
		}

		fullTops := f.getRoots()
		polTops := p.rootHashesForward()

		// check that tops match
		if len(fullTops) != len(polTops) {
			return fmt.Errorf("block %d full %d tops, pol %d tops",
				sn.blockHeight, len(fullTops), len(polTops))
		}
		fmt.Printf("top matching: ")
		for i, pt := range polTops {
			fmt.Printf("p %04x f %04x ", pt[:4],
				fullTops[i][:4])
			if pt != fullTops[i] {
				return fmt.Errorf("block %d top %d mismatch, full %x pol %x",
					sn.blockHeight, i, pt[:4],
					fullTops[i][:4])
			}
		}
		fmt.Printf("\n")
	}

	return nil
}

// fixedPollard adds and removes things in a non-random way
func fixedPollard(leaves int32) error {
	fmt.Printf("\t\tpollard test add %d remove 1\n", leaves)
	f := NewForest(nil, false, "", 0)

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

	err = p.rem2(dels)
	if err != nil {
		return err
	}

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

	f := NewForest(nil, false, "", 0)
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
			delete(leaves, del)
		}
		// add new leaves to the leaf map
		for _, leaf := range adds {
			fmt.Println("add", leaf)
			leaves[leaf.Hash] = leaf
		}

		for hash, l := range leaves {
			leafProof, err := f.ProveBatch([]Hash{hash})
			if err != nil {
				t.Fatal("Prove failed", err)
			}
			pos := leafProof.Targets[0]

			fmt.Println(pos, l)
			_, nsib, _, err := p.readPos(pos)

			if pos == p.numLeaves-1 {
				// roots are always cached
				continue
			}

			siblingDoesNotExists := nsib == nil || nsib.data == empty || err != nil
			if l.Remember && siblingDoesNotExists {
				// the proof for l is not cached even though it should have been because it
				// was added with remember=true.
				t.Fatal("proof for leaf at", pos, "does not exist but it was added with remember=true")
			} else if !l.Remember && !siblingDoesNotExists {
				// the proof for l was cached even though it should not have been because it
				// was added with remember = false.
				fmt.Println(p.ToString())
				t.Fatal("proof for leaf at", pos, "does exist but it was added with remember=false")
			}
		}
	}
}
