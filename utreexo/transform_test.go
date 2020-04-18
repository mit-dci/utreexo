package utreexo

import (
	"testing"
)

func TestExTwin(t *testing.T) {
	logger := NewLogger(t)
	logger.Printf("%d\n", topPos(15, 0, 4))

	dels := []uint64{0, 1, 2, 3, 9}

	parents, dels := ExTwin2(dels, 4)

	logger.Printf("parents %v dels %v\n", parents, dels)
}

func TestTopUp(t *testing.T) {
	logger := NewLogger(t)
	logger.Printf(BinString(15))

	rows := make([][]uint64, 5)
	rows[0] = []uint64{12}
	rows[1] = []uint64{21}
	// rows[2] = []uint64{23}
	topUp(rows, 4)

	logger.Printf("%v\n", rows)
}

func TestGetTop(t *testing.T) {
	logger := NewLogger(t)

	nl := uint64(11)
	h := uint8(1)
	top := topPos(nl, h, 4)

	logger.Printf("%d leaves, top at h %d is %d\n", nl, h, top)
}
