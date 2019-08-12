package utreexo

import (
	"fmt"
	"math/rand"
	"testing"
)

func TestUndoFixed(t *testing.T) {
	rand.Seed(6)
	err := undoAddOnly()
	if err != nil {
		t.Fatal(err)
	}

	// err := undoAddDel()
	// if err != nil {
	// t.Fatal(err)
	// }
}

func TestUndoRandom(t *testing.T) {
	rand.Seed(6)
	err := undoOneRandom(5)
	if err != nil {
		t.Fatal(err)
	}
}

func undoOneRandom(blocks int32) error {
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

	adds, delhashes = sc.NextBlock(2)

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

	lastTops := f.GetTops()

	// should be back to where it started
	fmt.Printf("tops: ")
	for i, lt := range lastTops {
		fmt.Printf("f %04x n %04x ", lt[:4], firstTops[i][:4])
		if lt != firstTops[i] {
			return fmt.Errorf("block %d top %d mismatch, full %x pol %x",
				sc.blockHeight, i, lt, firstTops[i])
		}
	}
	fmt.Printf("\n")

	return nil
}
