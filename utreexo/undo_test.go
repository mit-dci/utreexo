package utreexo

import (
	"fmt"
	"math/rand"
	"testing"
)

func TestUndoFixed(t *testing.T) {
	rand.Seed(6)
	//	err := undoAddOnly()
	//	if err != nil {
	//		t.Fatal(err)
	//	}
	fmt.Printf(BinString(16))
	err := undoAddDel()
	if err != nil {
		t.Fatal(err)
	}
}

func TestUndoRandom(t *testing.T) {
	rand.Seed(6)
	err := undoOnceRandom(5)
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

		// always build the undo data, even if we don't use it
		ub := f.BuildUndoData(adds, bp.Targets)
		fmt.Printf(ub.ToString())

		err = f.Modify(adds, bp.Targets)
		if err != nil {
			return err
		}

		//undo every 3rd block
		if b%3 == 0 {
			err := f.Undo(*ub)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func undoAddDel() error {
	f := NewForest()

	// p.Minleaves = 0

	sc := NewSimChain()
	sc.durationMask = 0x03
	adds, delHashes := sc.NextBlock(7)
	fmt.Printf("\t\tblock %d del %d add %d - %s\n",
		sc.blockHeight, len(delHashes), len(adds), f.Stats())

	bp, err := f.ProveBlock(delHashes)
	if err != nil {
		return err
	}

	err = f.Modify(adds, bp.Targets)
	if err != nil {
		return err
	}

	preTops := f.GetTops()
	for i, h := range preTops {
		fmt.Printf("pretop %d %x\n", i, h)
	}

	adds, delHashes = sc.NextBlock(0)
	fmt.Printf("\t\tblock %d del %d add %d - %s\n",
		sc.blockHeight, len(delHashes), len(adds), f.Stats())

	bp, err = f.ProveBlock(delHashes)
	if err != nil {
		return err
	}

	// before modifying, build undo data.
	// Should this happen automatically with forest.Modify..?
	ub := f.BuildUndoData(adds, bp.Targets)
	fmt.Printf(ub.ToString())

	err = f.Modify(adds, bp.Targets)
	if err != nil {
		return err
	}
	intermediateTops := f.GetTops()
	for i, h := range intermediateTops {
		fmt.Printf("intermediateTops %d %x\n", i, h)
	}

	err = f.Undo(*ub)
	if err != nil {
		return err
	}

	postTops := f.GetTops()
	for i, h := range postTops {
		fmt.Printf("posttop %d %x\n", i, h)
	}
	fmt.Printf("tops: ")
	for i, _ := range preTops {
		fmt.Printf("pre %04x post %04x ", preTops[i][:4], postTops[i][:4])
		if postTops[i] != preTops[i] {
			return fmt.Errorf("block %d top %d mismatch, pre %x post %x",
				sc.blockHeight, i, preTops[i][:4], postTops[i][:4])
		}
	}
	fmt.Printf("\n")

	return nil
}

func undoAddOnly() error {
	f := NewForest()

	sc := NewSimChain()
	sc.durationMask = 0
	adds, delhashes := sc.NextBlock(6)

	bp, err := f.ProveBlock(delhashes)
	if err != nil {
		return err
	}

	err = f.Modify(adds, bp.Targets)
	if err != nil {
		return err
	}

	preTops := f.GetTops()

	adds, delhashes = sc.NextBlock(0)

	bp, err = f.ProveBlock(delhashes)
	if err != nil {
		return err
	}
	ub := f.BuildUndoData(adds, bp.Targets)
	fmt.Printf(ub.ToString())

	err = f.Modify(adds, bp.Targets)
	if err != nil {
		return err
	}

	err = f.Undo(*ub)
	if err != nil {
		return err
	}
	fmt.Printf("post undo %s", f.ToString())

	postTops := f.GetTops()

	// should be back to where it started
	fmt.Printf("tops: ")
	for i, _ := range preTops {
		fmt.Printf("pre %04x post %04x ", preTops[i][:4], postTops[i][:4])
		if postTops[i] != postTops[i] {
			return fmt.Errorf("block %d top %d mismatch, pre %x post %x",
				sc.blockHeight, i, preTops[i][:4], postTops[i][:4])
		}
	}

	fmt.Printf("\n")

	return nil
}
