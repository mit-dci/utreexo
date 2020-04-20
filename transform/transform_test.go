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
	fmt.Println(util.BinString(15))

	rows := make([][]uint64, 5)
	rows[0] = []uint64{12}
	rows[1] = []uint64{21}
	// rows[2] = []uint64{23}
	util.TopUp(rows, 4)

	fmt.Printf("%v\n", rows)
}

func TestTopPos(t *testing.T) {
	for numLeaves := uint64(0); numLeaves < 20; numLeaves++ {
		//numLeaves := uint64(7)
		height := util.TreeHeight(numLeaves)
		for row := uint8(0); row <= height; row++ {
			//row := uint8(2)
			top := util.TopPos(numLeaves, row, height)
			fmt.Printf("%d leaves, %d height, top at row %d is %d\n",
				numLeaves, height, row, top)
		}
	}
}
