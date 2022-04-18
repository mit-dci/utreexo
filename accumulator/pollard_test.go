package accumulator

import (
	"encoding/hex"
	"fmt"
	"math/rand"
	"testing"
)

// checkHashes moves down the tree and calculates the parent hash from the children.
// It errors if the calculated hash doesn't match the hash found in the pollard.
func checkHashes(node, sibling *polNode, p *Pollard) error {
	// If node has a niece, then we can calculate the hash of the sibling because
	// every tree is a perfect binary tree.
	if node.leftNiece != nil {
		calculated := parentHash(node.leftNiece.data, node.rightNiece.data)
		if sibling.data != calculated {
			return fmt.Errorf("For position %d, calculated %s from left %s, right %s but read %s",
				sibling.calculatePosition(p.numLeaves, p.roots),
				hex.EncodeToString(calculated[:]),
				hex.EncodeToString(node.leftNiece.data[:]), hex.EncodeToString(node.rightNiece.data[:]),
				hex.EncodeToString(sibling.data[:]))
		}

		err := checkHashes(node.leftNiece, node.rightNiece, p)
		if err != nil {
			return err
		}
	}

	if sibling.leftNiece != nil {
		calculated := parentHash(sibling.leftNiece.data, sibling.rightNiece.data)
		if node.data != calculated {
			return fmt.Errorf("For position %d, calculated %s from left %s, right %s but read %s",
				node.calculatePosition(p.numLeaves, p.roots),
				hex.EncodeToString(calculated[:]),
				hex.EncodeToString(sibling.leftNiece.data[:]), hex.EncodeToString(sibling.rightNiece.data[:]),
				hex.EncodeToString(node.data[:]))
		}

		err := checkHashes(sibling.leftNiece, sibling.rightNiece, p)
		if err != nil {
			return err
		}
	}

	return nil
}

func TestPollardAdd(t *testing.T) {
	// simulate blocks with simchain
	numAdds := uint32(300)
	sc := newSimChain(0x07)

	var p Pollard
	p.MakeFull()

	for b := 0; b < 2000; b++ {
		fmt.Println("on block", b)
		adds, _, _ := sc.NextBlock(numAdds)

		err := p.ModifySwapless(adds, nil)
		if err != nil {
			t.Fatalf("TestSwapLessAddDel fail at block %d. Error: %v", b, err)
		}

		if b%100 == 0 {
			for _, root := range p.roots {
				if root.leftNiece != nil && root.rightNiece != nil {
					err = checkHashes(root.leftNiece, root.rightNiece, &p)
					if err != nil {
						t.Fatal(err)
					}
				}
			}
		}
	}
}

func TestPollardAddDel(t *testing.T) {
	// simulate blocks with simchain
	numAdds := uint32(150)
	sc := newSimChain(0x07)

	var p Pollard
	p.MakeFull()

	for b := 0; b < 2000; b++ {
		fmt.Println("on block", b)
		adds, _, delHashes := sc.NextBlock(numAdds)

		bp, err := p.ProveBatchSwapless(delHashes)
		if err != nil {
			t.Fatalf("TestSwapLessAddDel fail at block %d. Error: %v", b, err)
		}

		err = p.Verify(delHashes, bp)
		if err != nil {
			t.Fatal(err)
		}

		//err = p.VerifyCached(delHashes, bp)
		//if err != nil {
		//	t.Fatal(err)
		//}

		//fmt.Println(bp.ToString())

		for _, target := range bp.Targets {
			n, _, _, err := p.readPos(target)
			if err != nil {
				t.Fatalf("TestSwapLessAddDel fail at block %d. Error: %v", b, err)
			}
			if n == nil {
				fmt.Println(bp.ToString())
				t.Fatalf("TestSwapLessAddDel fail to read %d at block %d.", target, b)
			}
			//fmt.Printf("read %s at pos %d\n", hex.EncodeToString(n.data[:]), target)
		}

		err = p.ModifySwapless(adds, bp.Targets)
		if err != nil {
			t.Fatalf("TestSwapLessAddDel fail at block %d. Error: %v", b, err)
		}

		for _, root := range p.roots {
			if root.leftNiece != nil && root.rightNiece != nil {
				err = checkHashes(root.leftNiece, root.rightNiece, &p)
				if err != nil {
					t.Fatal(err)
				}
			}
		}

		//fmt.Println(p.ToString())
	}
}

func TestPollardProveSwapless(t *testing.T) {
	var tests = []struct {
		leaves   []Leaf
		dels     []Hash
		expected BatchProof
	}{
		{
			[]Leaf{
				{Hash{1}, false},
				{Hash{2}, false},
				{Hash{3}, false},
				{Hash{4}, false},
				{Hash{5}, true},
				{Hash{6}, false},
				{Hash{7}, false},
				{Hash{8}, false},
				{Hash{9}, false},
				{Hash{10}, false},
				{Hash{11}, false},
				{Hash{12}, false},
				{Hash{13}, false},
				{Hash{14}, false},
				{Hash{15}, false},
				{Hash{16}, false},
				{Hash{17}, false},
			},
			[]Hash{{5}, {6}, {7}, {9}},
			BatchProof{},
		},
	}

	for _, test := range tests {
		var p, fullP Pollard
		fullP.MakeFull()
		p.NodeMap = make(map[MiniHash]*polNode)

		err := p.ModifySwapless(test.leaves, nil)
		if err != nil {
			t.Fatal(err)
		}

		err = fullP.ModifySwapless(test.leaves, nil)
		if err != nil {
			t.Fatal(err)
		}

		fmt.Println("pol", p.ToString())

		fmt.Println("fullpol", fullP.ToString())

		bp, err := fullP.ProveBatchSwapless(test.dels)
		if err != nil {
			t.Fatal(err)
		}

		err = p.VerifyCached(test.dels, bp)
		if err != nil {
			t.Fatal(err)
		}

		fmt.Println("bp", bp.ToString())
		bp.Proof[2] = Hash{1}
		fmt.Println("modified bp", bp.ToString())

		err = p.VerifyCached(test.dels, bp)
		if err == nil {
			err := fmt.Errorf("Modified Proof passed the Verify check")
			t.Fatal(err)
		}

		bp, err = fullP.ProveBatchSwapless([]Hash{{17}})
		if err != nil {
			t.Fatal(err)
		}

		fmt.Println(bp.ToString())

		err = p.Verify([]Hash{{17}}, bp)
		if err != nil {
			t.Fatal(err)
		}
	}

}

func TestPollardAddSwapless(t *testing.T) {
	var p Pollard
	p.NodeMap = make(map[MiniHash]*polNode)
	//p.MakeFull()

	// Create the starting off pollard.
	adds := make([]Leaf, 6)
	for i := 0; i < len(adds); i++ {
		adds[i].Hash[0] = uint8(i)
		adds[i].Hash[20] = 0xff
		adds[i].Remember = true
	}

	p.addSwapless(adds)

	fmt.Println(p.ToString())

	//err := p.removeSwapless([]uint64{4, 5, 6, 8, 9, 10, 11, 12, 13, 14})
	//if err != nil {
	//	t.Fatal(err)
	//}

	//fmt.Println(p.ToString())

	//bp, err := p.ProveBatch([]Hash{{1}, {2}})
	//if err != nil {
	//	t.Fatal(err)
	//}

	//fmt.Println(bp.ToString())

	n, _, _, err := p.readPos(5)
	if err != nil {
		t.Fatal(err)
	}

	//fmt.Printf("n %s\n", hex.EncodeToString(n.data[:]))
	//fmt.Printf("aunt %s\n", hex.EncodeToString(n.aunt.data[:]))

	//fmt.Printf("niece[0] %s\n", hex.EncodeToString(n.aunt.niece[0].data[:]))
	//fmt.Printf("niece[1] %s\n", hex.EncodeToString(n.aunt.niece[1].data[:]))

	//fmt.Printf("n.aunt.aunt %s\n", hex.EncodeToString(n.aunt.aunt.data[:]))
	//fmt.Printf("n.aunt.aunt.aunt %s\n", hex.EncodeToString(n.aunt.aunt.aunt.data[:]))
	//fmt.Printf("n.aunt.aunt.aunt.aunt %s\n", hex.EncodeToString(n.aunt.aunt.aunt.aunt.data[:]))

	pos := n.calculatePosition(p.numLeaves, p.roots)
	fmt.Println("got pos", pos)

	//p.addSwapless([]Leaf{{Hash{7}, false}})

	//fmt.Println(p.ToString())

	fmt.Println("node map len", len(p.NodeMap))

	err = p.removeSwapless([]uint64{5})
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println(p.ToString())

	for key, value := range p.NodeMap {
		fmt.Println("hi")
		fmt.Printf("hash %s, node pos %d\n", hex.EncodeToString(key[:]),
			value.calculatePosition(p.numLeaves, p.roots))
	}

	node, ok := p.NodeMap[Hash{4}.Mini()]
	if !ok {
		hash := Hash{4}
		t.Fatalf("couldn't find %s", hex.EncodeToString(hash[:]))
	}

	fmt.Println(node.data)

	//p.addSwapless([]Leaf{{Hash{8}, true}, {Hash{9}, true}})

	//fmt.Println(p.ToString())

	//err = p.removeSwapless([]uint64{9})
	//if err != nil {
	//	t.Fatal(err)
	//}

	p.addSwapless([]Leaf{
		{Hash{6}, true},
		{Hash{7}, true},
		//{Hash{6}, true},
		//{Hash{7}, true},
	})

	fmt.Println(p.ToString())

	p.removeSwapless([]uint64{0})
	fmt.Println(p.ToString())

	for _, root := range p.roots {
		if root.leftNiece != nil && root.rightNiece != nil {
			err = checkHashes(root.leftNiece, root.rightNiece, &p)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestPollardNoSiblingFound(t *testing.T) {
	var p Pollard

	// Create the starting off pollard.
	adds := make([]Leaf, 7)
	for i := 0; i < len(adds); i++ {
		adds[i].Hash[0] = uint8(i)
		adds[i].Hash[20] = 0xff
		adds[i].Remember = true
	}
	adds[6].Remember = false

	err := p.Modify(adds, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Create the next adds and dels that gets us to the stage right
	// before the swapNodes error.
	newAdds := make([]Leaf, 4)
	for i := 0; i < len(newAdds); i++ {
		newAdds[i].Hash[0] = uint8(i)
		newAdds[i].Hash[1] = uint8(2)
		newAdds[i].Hash[2] = uint8(7)
		newAdds[i].Hash[20] = 0xff
	}

	newAdds[0].Remember = true
	newAdds[1].Remember = true

	dels := []uint64{2, 3, 4}
	err = p.Modify(newAdds, dels)
	if err != nil {
		t.Fatal(err)
	}

	// Then cause the error by deleting 1,3,4,5
	newDels := []uint64{1, 3, 4, 5}
	err = p.Modify(nil, newDels)
	if err != nil {
		t.Fatal(err)
	}
}

func TestPollardRand(t *testing.T) {
	for z := 0; z < 30; z++ {
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
	err := p.IngestBatchProof(hashes, bp, false)
	if err == nil {
		t.Fatal("BatchProof valid after modification. Accumulator validation failing")
	}
}

func pollardRandomRemember(blocks int32) error {
	f := NewForest(RamForest, nil, "", 0)

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
		err = p.IngestBatchProof(delHashes, bp, false)
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

		fullTops := f.GetRoots()
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

// TestPollardIngestMultiBlockProof tests that a pollard is able to ingest a
// proof for a range of blocks and then be able to verify all the deletions
// that happen later.
func TestPollardIngestMultiBlockProof(t *testing.T) {
	var (
		p Pollard

		// Where the previous blocks' adds and dels will be stored.
		prevAdds [][]Leaf
		prevDels [][]Hash

		// Interval in which proof will be generated. If the interval is 3,
		// then proofs will be generated at blocks 3, 6, 9, 12...
		proofGenerationInterVal int32 = 4

		// Total amount of blocks the test will generate and process.
		blocks int32 = 4000
	)

	f := NewForest(RamForest, nil, "", 0)

	// When generating proofs in intervals of blocks, the addtions that
	// get created in-between an interval will not be proven.
	doNotProveMap := make(map[Hash]struct{})

	// Create the chain to interate over.
	sn := newSimChain(0x07)

	// Remember all the leaves within an interval as the multi-block proof will
	// not contain proofs for those.
	sn.lookahead = proofGenerationInterVal
	for b := int32(0); b < blocks; b++ {
		adds, _, delHashes := sn.NextBlock(3)

		// All adds get added to the map so that they will not be attempted
		// to be proven. Not doing so will result in an error for forest because
		// it can't prove these.
		for _, add := range adds {
			doNotProveMap[add.Hash] = struct{}{}
		}

		// If we're NOT on the genesis block and are not on the interval
		// where proofs should be generated, just append the adds and dels
		// and move onto the next block.
		//
		// If we are on the interval where proofs should be generated, append
		// the current block's additions and deletions and then generate proofs.
		if b == 0 || b%proofGenerationInterVal != 0 {
			prevAdds = append(prevAdds, adds)
			prevDels = append(prevDels, delHashes)

			continue
		} else {
			prevAdds = append(prevAdds, adds)
			prevDels = append(prevDels, delHashes)
		}

		// Create a single array of all the deletions to be proven.
		var delsToProve []Hash
		for _, dels := range prevDels {
			for _, del := range dels {
				// Add to the array to be proven only if this deletion
				// is not in the doNotProveMap.
				_, found := doNotProveMap[del]
				if !found {
					delsToProve = append(delsToProve, del)
				}
			}
		}

		multiBlockBP, err := f.ProveBatch(delsToProve)
		if err != nil {
			t.Fatal(fmt.Errorf("Couldn't prove the multi-block deletions. Error: %s",
				err.Error()))
		}
		p.PruneAll()

		// IngestBatchProof with rememberAll as true as all the proof here will be
		// needed with later blocks.
		err = p.IngestBatchProof(delsToProve, multiBlockBP, true)
		if err != nil {
			t.Fatal(fmt.Errorf("Couldn't ingest the multi-block proof. Error: %s",
				err.Error()))
		}

		// Go through all the previous addtions and deletions that happened at
		// each block and replay them.
		for i, prevDel := range prevDels {
			bp, err := f.ProveBatch(prevDel)
			if err != nil {
				t.Fatal(fmt.Errorf("Couldn't prove deletions. Error: %s",
					err.Error()))
			}

			err = f.sanity()
			if err != nil {
				t.Fatal(fmt.Errorf("Forest sanity failed. Error: %s", err.Error()))
			}

			err = f.PosMapSanity()
			if err != nil {
				t.Fatal(fmt.Errorf("Forest position map sanity failed. Error: %s",
					err.Error()))
			}

			loopAdd := prevAdds[i]

			_, err = f.Modify(loopAdd, bp.Targets)
			if err != nil {
				t.Fatal(fmt.Errorf("Forest modify failed. Error: %s",
					err.Error()))
			}

			// Apply adds and dels to the pollard.
			err = p.Modify(loopAdd, bp.Targets)
			if err != nil {
				t.Fatal(fmt.Errorf("Pollard modify failed. Error: %s",
					err.Error()))
			}

			// Check all leaves match with the forest.
			if !p.equalToForestIfThere(f) {
				t.Fatal(fmt.Errorf("Pollard and forest leaves differ"))
			}

			forestRoots := f.GetRoots()
			pollardRoots := p.rootHashesForward()

			// Check that roots match.
			if len(forestRoots) != len(pollardRoots) {
				t.Fatal(fmt.Errorf("Pollard has %d roots but forest has %d roots at block %d",
					len(pollardRoots), len(forestRoots), sn.blockHeight))
			}

			for i, pollardRoot := range pollardRoots {
				forestRoot := forestRoots[i]
				if pollardRoot != forestRoot {
					t.Fatal(fmt.Errorf("Root mismatch. Pollard root is %s "+
						"but forest root is %s at block %d",
						hex.EncodeToString(pollardRoot[:]),
						hex.EncodeToString(forestRoot[:]),
						sn.blockHeight))
				}
			}
		}

		// Reset all the previous blocks' additions and deletions
		prevAdds = prevAdds[:0]
		prevDels = prevDels[:0]

		// Reset the map as well.
		doNotProveMap = make(map[Hash]struct{})
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

		err = p.IngestBatchProof(delHashes, proof, false)
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

			parent, _, _, err := p.readPos(pos)
			if err != nil {
				t.Fatal(fmt.Errorf("Couldn't read parent position of %d. err: %v", pos, err))
			}

			if l.Remember && parent == nil {
				fmt.Println(p.ToString())
				err := fmt.Errorf("leaf at position %d exists but it was added with "+
					"remember=%v and its parent does not exist. "+
					"polnode remember=%v",
					pos, l.Remember, n.remember)
				t.Fatal(err)
			}
		}
		fmt.Println(p.ToString())
	}
}
