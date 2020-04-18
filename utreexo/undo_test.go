package utreexo

import (
	"fmt"
	"log"
	"math/rand"
	"testing"
)

func TestUndoFixed(t *testing.T) {
	logger := NewLogger(t)
	rand.Seed(2)

	// needs in-order
	err := undoAddDelOnce(logger, 6, 4, 4)
	if err != nil {
		t.Fatal(err)
	}
}

func TestUndoRandom(t *testing.T) {
	logger := NewLogger(t)

	for z := int64(0); z < 100; z++ {
		// z := int64(11)
		rand.Seed(z)
		err := undoOnceRandom(logger, 20)
		if err != nil {
			logger.Printf("rand seed %d\n", z)
			t.Fatal(err)
		}
	}
}

func TestUndoTest(t *testing.T) {
	logger := NewLogger(t)
	rand.Seed(1)
	err := undoTestSimChain(logger)
	if err != nil {
		t.Fatal(err)
	}
}

func undoOnceRandom(logger *log.Logger, blocks int32) error {
	f := NewForest(nil)

	sc := NewSimChain(0x07)
	sc.lookahead = 0
	for b := int32(0); b < blocks; b++ {

		adds, delHashes := sc.NextBlock(rand.Uint32() & 0x03)

		logger.Printf("\t\tblock %d del %d add %d - %s\n",
			sc.blockHeight, len(delHashes), len(adds), f.Stats())

		bp, err := f.ProveBlock(delHashes)
		if err != nil {
			return err
		}

		ub, err := f.Modify(adds, bp.Targets)
		if err != nil {
			return err
		}
		logger.Printf(f.ToString())
		logger.Printf(sc.ttlString())
		for h, p := range f.positionMap {
			logger.Printf("%x@%d ", h[:4], p)
		}
		err = f.PosMapSanity()
		if err != nil {
			return err
		}

		//undo every 3rd block
		if b%3 == 2 {
			logger.Printf(ub.ToString())
			err := f.Undo(*ub)
			if err != nil {
				return err
			}
			logger.Printf("\n post undo map: ")
			for h, p := range f.positionMap {
				logger.Printf("%x@%d ", h[:4], p)
			}
			sc.BackOne(adds, delHashes)
		}

	}
	return nil
}

func undoAddDelOnce(logger *log.Logger, numStart, numAdds, numDels uint32) error {
	f := NewForest(nil)
	sc := NewSimChain(0xff)

	// --------------- block 0
	// make starting forest with numStart leaves, and store tops
	adds, _ := sc.NextBlock(numStart)
	logger.Printf("adding %d leaves\n", numStart)
	_, err := f.Modify(adds, nil)
	if err != nil {
		return err
	}
	logger.Printf(f.ToString())
	beforeTops := f.GetTops()
	for i, h := range beforeTops {
		logger.Printf("beforeTops %d %x\n", i, h)
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

	logger.Printf("block 1 add %d rem %d\n", numAdds, numDels)

	ub, err := f.Modify(adds, bp.Targets)
	if err != nil {
		return err
	}
	logger.Printf(f.ToString())
	logger.Printf(ub.ToString())
	afterTops := f.GetTops()
	for i, h := range afterTops {
		logger.Printf("afterTops %d %x\n", i, h)
	}

	err = f.Undo(*ub)
	if err != nil {
		return err
	}

	undoneTops := f.GetTops()
	for i, h := range undoneTops {
		logger.Printf("undoneTops %d %x\n", i, h)
	}
	for h, p := range f.positionMap {
		logger.Printf("%x@%d ", h[:4], p)
	}
	logger.Printf("tops: ")
	for i, _ := range beforeTops {
		logger.Printf("pre %04x post %04x ", beforeTops[i][:4], undoneTops[i][:4])
		if undoneTops[i] != beforeTops[i] {
			return fmt.Errorf("block %d top %d mismatch, pre %x post %x",
				sc.blockHeight, i, beforeTops[i][:4], undoneTops[i][:4])
		}
	}
	logger.Printf("\n")

	return nil
}

func undoTestSimChain(logger *log.Logger) error {

	sc := NewSimChain(7)
	sc.NextBlock(3)
	sc.NextBlock(3)
	sc.NextBlock(3)
	logger.Printf("\n" + sc.ttlString())
	l1, h1 := sc.NextBlock(3)
	logger.Printf("\n" + sc.ttlString())
	sc.BackOne(l1, h1)
	logger.Printf("\n" + sc.ttlString())
	return nil
}
