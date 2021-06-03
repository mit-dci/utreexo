package accumulator

import (
	"fmt"
	"math/rand"
	"testing"
)

func TestUndoFixed(t *testing.T) {
	rand.Seed(2)

	// needs in-order
	err := undoAddDelOnce(6, 4, 4)
	if err != nil {
		t.Fatal(err)
	}
}

func TestUndoRandom(t *testing.T) {

	for z := int64(0); z < 100; z++ {
		// z := int64(11)
		rand.Seed(z)
		err := undoOnceRandom(20)
		if err != nil {
			fmt.Printf("rand seed %d\n", z)
			t.Fatal(err)
		}
	}
}

func TestUndoTest(t *testing.T) {
	rand.Seed(1)
	err := undoTestSimChain()
	if err != nil {
		t.Fatal(err)
	}
}

func undoOnceRandom(blocks int32) error {
	f := NewForest(nil, false, "", 0)

	sc := newSimChain(0x07)
	sc.lookahead = 0
	for b := int32(0); b < blocks; b++ {

		adds, durations, delHashes := sc.NextBlock(rand.Uint32() & 0x03)

		fmt.Printf("\t\tblock %d del %d add %d - %s\n",
			sc.blockHeight, len(delHashes), len(adds), f.Stats())

		bp, err := f.ProveBatch(delHashes)
		if err != nil {
			return err
		}
		ub, err := f.Modify(adds, bp.Targets)
		if err != nil {
			return err
		}
		fmt.Print(f.ToString())
		fmt.Print(sc.ttlString())
		for h, p := range f.positionMap {
			fmt.Printf("%x@%d ", h[:4], p)
		}
		err = f.PosMapSanity()
		if err != nil {
			return err
		}

		//undo every 3rd block
		if b%3 == 2 {
			fmt.Print(ub.ToString())
			err := f.Undo(*ub)
			if err != nil {
				return err
			}
			fmt.Print("\n post undo map: ")
			for h, p := range f.positionMap {
				fmt.Printf("%x@%d ", h[:4], p)
			}
			sc.BackOne(adds, durations, delHashes)
		}

	}
	return nil
}

func undoAddDelOnce(numStart, numAdds, numDels uint32) error {
	f := NewForest(nil, false, "", 0)
	sc := newSimChain(0xff)

	// --------------- block 0
	// make starting forest with numStart leaves, and store tops
	adds, _, _ := sc.NextBlock(numStart)
	fmt.Printf("adding %d leaves\n", numStart)
	_, err := f.Modify(adds, nil)
	if err != nil {
		return err
	}
	fmt.Printf(f.ToString())
	beforeTops := f.getRoots()
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
	adds, _, _ = sc.NextBlock(numAdds)

	bp, err := f.ProveBatch(delHashes)
	if err != nil {
		return err
	}

	fmt.Printf("block 1 add %d rem %d\n", numAdds, numDels)

	ub, err := f.Modify(adds, bp.Targets)
	if err != nil {
		return err
	}
	fmt.Print(f.ToString())
	fmt.Print(ub.ToString())
	afterTops := f.getRoots()
	for i, h := range afterTops {
		fmt.Printf("afterTops %d %x\n", i, h)
	}

	err = f.Undo(*ub)
	if err != nil {
		return err
	}

	undoneTops := f.getRoots()
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

func undoTestSimChain() error {

	sc := newSimChain(7)
	sc.NextBlock(3)
	sc.NextBlock(3)
	sc.NextBlock(3)
	fmt.Printf(sc.ttlString())
	l1, dur, h1 := sc.NextBlock(3)
	fmt.Printf(sc.ttlString())
	sc.BackOne(l1, dur, h1)
	fmt.Printf(sc.ttlString())
	return nil
}
