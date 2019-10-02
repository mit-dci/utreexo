package utreexo

import (
	"fmt"
	"testing"
)

func TestTopDown(t *testing.T) {

	// mv, stash := removeTransform([]uint64{1}, 16, 4)
	// fmt.Printf("mv %v, stash %v\n", mv, stash)
	// arrows := mergeAndReverseArrows(mv, stash)
	// td := topDown(arrows, 4)
	// fmt.Printf("td %v\n", td)

	fup := NewForest()   // bottom up modified forest
	fdown := NewForest() // top down modified forest
	// ideally they are the same

	adds := make([]LeafTXO, 10)
	for j := range adds {
		adds[j].Hash[0] = uint8(j) | 0xa0
	}

	fup.Modify(adds, nil)
	fdown.Modify(adds, nil)

	// fmt.Printf(fup.ToString())
	// fmt.Printf(fdown.ToString())

	//initial state
	fmt.Printf(fup.ToString())

	dels := []uint64{0, 1, 2, 3, 4}

	err := fup.removev2(dels)
	if err != nil {
		t.Fatal(err)
	}
	err = fdown.removev3(dels)
	if err != nil {
		t.Fatal(err)
	}

	upTops := fup.GetTops()
	downTops := fdown.GetTops()

	fmt.Printf("up nl %d %s", fup.numLeaves, fup.ToString())
	fmt.Printf("down nl %d %s", fdown.numLeaves, fdown.ToString())

	if len(upTops) != len(downTops) {
		t.Fatalf("tops mismatch up %d down %d\n", len(upTops), len(downTops))
	}
	for i, _ := range upTops {
		fmt.Printf("up %04x down %04x ", upTops[i][:4], downTops[i][:4])
		if downTops[i] != upTops[i] {
			t.Fatalf("forest mismatch, up %x down %x",
				upTops[i][:4], downTops[i][:4])
		}
	}

}

func TestIsDescendant(t *testing.T) {
	/*
		will test with a 4-high tree
		   30
		   |-------------------------------\
		   28                              29
		   |---------------\               |---------------\
		   24              25              26              27
		   |-------\       |-------\       |-------\       |-------\
		   16      17      18      19      20      21      22      23
		   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\
		   00  01  02  03  04  05  06  07  08  09  10  11  12  13  14  15
	*/

	// 3 is under 24 but not 25
	p, a, b := 3, 24, 25
	aunder, bunder := isDescendant(uint64(p), uint64(a), uint64(b), 4)

	if !aunder || bunder {
		t.Fatalf("isDescendant %v %v error for %d under %d %d",
			aunder, bunder, p, a, b)
	}

	p, a, b = 20, 28, 29

	aunder, bunder = isDescendant(uint64(p), uint64(a), uint64(b), 4)
	if aunder || !bunder {
		t.Fatalf("isDescendant %v %v error for %d under %d %d",
			aunder, bunder, p, a, b)
	}

}
