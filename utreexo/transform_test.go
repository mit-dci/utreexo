package utreexo

import (
	"fmt"
	"testing"
)

func TestTopDown(t *testing.T) {

	mv, stash := removeTransform([]uint64{0, 2}, 16, 4)
	fmt.Printf("mv %v, stash %v\n", mv, stash)
	arrows := mergeAndReverseArrows(mv, stash)
	td := topDown(arrows, 4)
	fmt.Printf("td %v\n", td)
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
