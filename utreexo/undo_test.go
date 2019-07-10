package utreexo

import (
	"fmt"
	"math/rand"
	"testing"
)

func TestUndo(t *testing.T) {
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
	err := undofixed()
	if err != nil {
		t.Fatal(err)
	}
	//	}

}

func undofixed() error {
	f := NewForest()

	bu := new(blockUndo)

	// p.Minleaves = 0

	sc := NewSimChain()

	adds, delhashes := sc.NextBlock(5)

	bp, err := f.ProveBlock(delhashes)
	if err != nil {
		return err
	}

	_, err = f.Modify(adds, bp.Targets)
	if err != nil {
		return err
	}

	firstTops := f.GetTops()

	adds, delhashes = sc.NextBlock(5)

	bp, err = f.ProveBlock(delhashes)
	if err != nil {
		return err
	}

	bu, err = f.Modify(adds, bp.Targets)
	if err != nil {
		return err
	}

	err = f.Undo(*bu)
	if err != nil {
		return err
	}

	lastTops := f.GetTops()

	fmt.Printf("firstTops %v\nlastTops %v\n", firstTops, lastTops)
	return nil
}
