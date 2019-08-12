package utreexo

import (
	"fmt"
	"math/rand"
	"testing"
)

func TestUndo(t *testing.T) {
	rand.Seed(6)
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
	err := undoAddDel()
	if err != nil {
		t.Fatal(err)
	}
	//	}

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

	firstTops := f.GetTops()

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

	err = f.Undo(*ub)
	if err != nil {
		return err
	}

	lastTops := f.GetTops()

	fmt.Printf("tops: ")
	for i, lt := range lastTops {
		fmt.Printf("f %04x n %04x ", lt[:4], firstTops[i][:4])
		// if lt != firstTops[i] {
		// return fmt.Errorf("block %d top %d mismatch, full %x pol %x",
		// sc.blockHeight, i, lt, firstTops[i])
		// }
	}
	fmt.Printf("\n")

	return nil
}

func undoAddOnly() error {
	f := NewForest()

	ub := new(undoBlock)

	// p.Minleaves = 0

	sc := NewSimChain()
	sc.durationMask = 0
	adds, delhashes := sc.NextBlock(5)

	bp, err := f.ProveBlock(delhashes)
	if err != nil {
		return err
	}

	err = f.Modify(adds, bp.Targets)
	if err != nil {
		return err
	}

	firstTops := f.GetTops()

	adds, delhashes = sc.NextBlock(5)

	bp, err = f.ProveBlock(delhashes)
	if err != nil {
		return err
	}

	err = f.Modify(adds, bp.Targets)
	if err != nil {
		return err
	}

	err = f.Undo(*ub)
	if err != nil {
		return err
	}

	lastTops := f.GetTops()

	fmt.Printf("tops: ")
	for i, lt := range lastTops {
		fmt.Printf("f %04x n %04x ", lt[:4], firstTops[i][:4])
		// if lt != firstTops[i] {
		// return fmt.Errorf("block %d top %d mismatch, full %x pol %x",
		// sc.blockHeight, i, lt, firstTops[i])
		// }
	}
	fmt.Printf("\n")

	return nil
}
