package accumulator

import (
	"fmt"
	"testing"
)

func TestExTwin(t *testing.T) {

	fmt.Printf("%d\n", rootPosition(15, 0, 4))

	dels := []uint64{0, 1, 2, 3, 9}

	parents, dels := extractTwins(dels, 4)

	fmt.Printf("parents %v dels %v\n", parents, dels)
}

func TestGetTop(t *testing.T) {

	nl := uint64(11)
	h := uint8(1)
	root := rootPosition(nl, h, 4)

	fmt.Printf("%d leaves, top at h %d is %d\n", nl, h, root)
}
