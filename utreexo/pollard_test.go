package utreexo

import (
	"fmt"
	"math/rand"
	"testing"
)

func TestRandPollard(t *testing.T) {
	rand.Seed(9)
	//	err := pollardMiscTest()
	//	if err != nil {
	//		t.Fatal(err)
	//	}
	//	for i := 6; i < 100; i++ {
	//	err := fixedPollard(15)
	//	if err != nil {
	//		t.Fatal(err)
	//	}

	//	for z := 0; z < 100; z++ {
	err := pollardRandomRemember(9)
	if err != nil {
		t.Fatal(err)
	}
	//	}

}

func pollardRandomRemember(blocks int32) error {
	f := NewForest()

	var p Pollard

	// p.Minleaves = 0

	sn := NewSimChain(0x07)
	sn.lookahead = 400
	for b := int32(0); b < blocks; b++ {
		adds, delHashes := sn.NextBlock(rand.Uint32() & 0x03)

		fmt.Printf("\t\t\tstart block %d del %d add %d - %s\n",
			sn.blockHeight, len(delHashes), len(adds), p.Stats())

		// get proof for these deletions (with respect to prev block)
		bp, err := f.ProveBlock(delHashes)
		if err != nil {
			return err
		}

		// verify proofs on rad node
		err = p.IngestBlockProof(bp)
		if err != nil {
			return err
		}

		//		f2, err := p.toFull()
		//		if err != nil {
		//			return err
		//		}
		//		fmt.Printf("postingest %s", f2.ToString())
		// fmt.Printf("forgetslice %v leaf %v\n", p.forget, p.rememberLeaf)

		// apply adds and deletes to the bridge node (could do this whenever)
		_, err = f.Modify(adds, bp.Targets)
		if err != nil {
			return err
		}
		fmt.Printf("del %v\n", bp.Targets)
		// apply adds / dels to pollard
		err = p.Modify(adds, bp.Targets)
		if err != nil {
			return err
		}

		f2, err := p.toFull()
		if err != nil {
			return err
		}

		fmt.Printf("pol postadd %s", f2.ToString())
		//		fmt.Printf("forgetslice %v\n", p.forget)
		// if p.rememberLeaf {
		//			return fmt.Errorf("nobody should be memorable")
		// }
		// check that tops are the same

		fullTops := f.GetTops()
		polTops := p.topHashesReverse()

		// check that tops match
		if len(fullTops) != len(polTops) {
			return fmt.Errorf("block %d full %d tops, pol %d tops",
				sn.blockHeight, len(fullTops), len(polTops))
		}
		fmt.Printf("top matching: ")
		for i, ft := range fullTops {
			fmt.Printf("f %04x p %04x ", ft[:4], polTops[i][:4])
			if ft != polTops[i] {
				return fmt.Errorf("block %d top %d mismatch, full %x pol %x",
					sn.blockHeight, i, ft, polTops[i])
			}
		}
		fmt.Printf("\n")
	}

	return nil
}

// fixedPollard adds and removes things in a non-random way
func fixedPollard(leaves int32) error {
	fmt.Printf("\t\tpollard test add %d remove 1\n", leaves)
	f := NewForest()

	leafCounter := uint64(0)

	dels := []uint64{0, 1, 7, 8, 10}

	// they're all forgettable
	adds := make([]LeafTXO, leaves)

	// make a bunch of unique adds & make an expiry time and add em to
	// the TTL map
	for j := range adds {
		adds[j].Hash[1] = uint8(leafCounter)
		adds[j].Hash[2] = uint8(leafCounter >> 8)
		adds[j].Hash[3] = uint8(leafCounter >> 16)
		adds[j].Hash[4] = uint8(leafCounter >> 24)
		adds[j].Hash[9] = uint8(0xff)

		// the first utxo addded lives forever.
		// (prevents leaves from goign to 0 which is buggy)

		leafCounter++
	}

	// apply adds and deletes to the bridge node (could do this whenever)
	_, err := f.Modify(adds, nil)
	if err != nil {
		return err
	}
	fmt.Printf(f.ToString())

	var p Pollard

	err = p.add(adds)
	if err != nil {
		return err
	}

	f2, err := p.toFull()
	if err != nil {
		return err
	}
	fmt.Printf(f2.ToString())

	err = p.rem(dels)
	if err != nil {
		return err
	}

	_, err = f.Modify(nil, dels)
	if err != nil {
		return err
	}
	fmt.Printf(f.ToString())

	f2, err = p.toFull()
	if err != nil {
		return err
	}
	fmt.Printf(f2.ToString())

	return nil
}
