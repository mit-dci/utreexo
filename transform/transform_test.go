package transform

import (
	"fmt"
	"testing"

	"github.com/mit-dci/utreexo/util"
)

func TestExTwin(t *testing.T) {

	fmt.Printf("%d\n", util.TopPos(15, 0, 4))

	dels := []uint64{0, 1, 2, 3, 9}

	parents, dels := util.ExTwin2(dels, 4)

	fmt.Printf("parents %v dels %v\n", parents, dels)
}

func TestTopUp(t *testing.T) {

	fmt.Printf(util.BinString(15))

	rows := make([][]uint64, 5)
	rows[0] = []uint64{12}
	rows[1] = []uint64{21}
	// rows[2] = []uint64{23}
	util.TopUp(rows, 4)

	fmt.Printf("%v\n", rows)
}

func TestGetTop(t *testing.T) {

	nl := uint64(11)
	h := uint8(1)
	top := util.TopPos(nl, h, 4)

	fmt.Printf("%d leaves, top at h %d is %d\n", nl, h, top)
}
