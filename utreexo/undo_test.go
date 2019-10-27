package utreexo

import (
	"fmt"
	"math/rand"
	"testing"
)

func TestUndoFixed(t *testing.T) {
	rand.Seed(2)

	// needs in-order
	err := undoAddDelOnce(15, 3, 3)
	if err != nil {
		t.Fatal(err)
	}
}

func TestUndoRandom(t *testing.T) {
	rand.Seed(9)
	err := undoOnceRandom(23)
	if err != nil {
		t.Fatal(err)
	}
}

func undoOnceRandom(blocks int32) error {
	f := NewForest()

	sc := NewSimChain()
	sc.durationMask = 0x07
	sc.lookahead = 0
	for b := int32(0); b < blocks; b++ {

		adds, delHashes := sc.NextBlock(rand.Uint32() & 0x03)

		fmt.Printf("\t\tblock %d del %d add %d - %s\n",
			sc.blockHeight, len(delHashes), len(adds), f.Stats())

		bp, err := f.ProveBlock(delHashes)
		if err != nil {
			return err
		}

		ub, err := f.Modify(adds, bp.Targets)
		if err != nil {
			return err
		}
		for h, p := range f.positionMap {
			fmt.Printf("%x@%d ", h[:4], p)
		}
		// always build the undo data, even if we don't use it
		// ub := f.BuildUndoData(adds, bp.Targets)
		fmt.Printf(ub.ToString())

		//undo every 3rd block
		if b%3 == 0 {
			err := f.Undo(*ub)
			if err != nil {
				return err
			}
		}
		fmt.Printf("\n post undo: ")
		for h, p := range f.positionMap {
			fmt.Printf("%x@%d ", h[:4], p)
		}
	}
	return nil
}

func undoAddDelOnce(numStart, numAdds, numDels uint32) error {
	f := NewForest()
	sc := NewSimChain()

	// --------------- block 0
	// make starting forest with numStart leaves, and store tops
	adds, _ := sc.NextBlock(numStart)
	_, err := f.Modify(adds, nil)
	if err != nil {
		return err
	}
	beforeTops := f.GetTops()
	for i, h := range beforeTops {
		fmt.Printf("beforeTops %d %x\n", i, h)
	}

	// ---------------- block 1
	// just delete from the left side for now.  Can try deleting scattered
	// randomly later
	delHashes := make([]Hash, numDels)
	for i, _ := range delHashes {
		delHashes[i] = adds[i].Hash
	}
	// get some more adds; the dels we already got
	adds, _ = sc.NextBlock(numAdds)

	bp, err := f.ProveBlock(delHashes)
	if err != nil {
		return err
	}

	ub, err := f.Modify(adds, bp.Targets)
	if err != nil {
		return err
	}
	fmt.Printf(ub.ToString())
	afterTops := f.GetTops()
	for i, h := range afterTops {
		fmt.Printf("afterTops %d %x\n", i, h)
	}

	err = f.Undov2(*ub)
	if err != nil {
		return err
	}

	undoneTops := f.GetTops()
	for i, h := range undoneTops {
		fmt.Printf("undoneTops %d %x\n", i, h)
	}
	for h, p := range f.positionMap {
		fmt.Printf("%x@%d ", h[:4], p)
	}
	fmt.Printf("tops: ")
	for i, _ := range beforeTops {
		fmt.Printf("pre %04x post %04x ", beforeTops[i][:4], undoneTops[i][:4])
		if undoneTops[i] != beforeTops[i] {
			return fmt.Errorf("block %d top %d mismatch, pre %x post %x",
				sc.blockHeight, i, beforeTops[i][:4], undoneTops[i][:4])
		}
	}
	fmt.Printf("\n")

	return nil
}
