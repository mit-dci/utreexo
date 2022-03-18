package accumulator

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"golang.org/x/exp/slices"
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

func TestTransform(t *testing.T) {
	var tests = []struct {
		forestRows uint8
		dels       []uint64
		expected   [][]arrow
	}{

		// 14
		// |---------------\
		// 12              13
		// |-------\       |-------\
		// 08      09      10      11
		// |---\   |---\   |---\   |---\
		// 00* 01* 02  03  04* 05  06  07
		{
			3,
			[]uint64{0, 1, 4},
			[][]arrow{
				[]arrow{{from: 5, to: 10}},
				[]arrow{{from: 9, to: 12}},
				[]arrow{},
				[]arrow{},
			},
		},

		// 14
		// |---------------\
		// 12              13
		// |-------\       |-------\
		// 08      09*     10      11*
		// |---\   |---\   |---\   |---\
		// 00* 01* 02  03  04* 05  06  07
		{
			3,
			[]uint64{0, 1, 4, 9, 11},
			[][]arrow{
				[]arrow{},
				[]arrow{},
				[]arrow{{from: 5, to: 14}},
				[]arrow{},
			},
		},

		// 30
		// |-------------------------------\
		// 28                              29
		// |---------------\               |---------------\
		// 24*             25*             26              27
		// |-------\       |-------\       |-------\       |-------\
		// 16      17      18      19      20      21      22      23
		// |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\
		// 00  01  02  03  04  05  06  07  08* 09* 10* 11  12  13  14  15
		{
			4,
			[]uint64{8, 9, 10, 24, 25},
			[][]arrow{
				[]arrow{},
				[]arrow{{from: 11, to: 26}},
				[]arrow{},
				[]arrow{{from: 29, to: 30}},
				[]arrow{},
			},
			//[][]arrow{
			//	[]arrow{{from: 11, to: 21}},
			//	[]arrow{{from: 21, to: 26}},
			//	[]arrow{},
			//	[]arrow{{from: 29, to: 30}},
			//	[]arrow{},
			//},
		},

		// 30
		// |-------------------------------\
		// 28                              29
		// |---------------\               |---------------\
		// 24              25              26              27*
		// |-------\       |-------\       |-------\       |-------\
		// 16      17      18*     19*     20      21*     22      23
		// |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\
		// 00* 01  02* 03  04  05  06  07  08  09  10  11  12  13  14  15
		{
			4,
			[]uint64{0, 2, 18, 19, 21, 27},
			[][]arrow{
				[]arrow{{from: 01, to: 16}, {from: 3, to: 17}},
				[]arrow{},
				[]arrow{{from: 24, to: 28}, {from: 20, to: 29}},
				[]arrow{},
				[]arrow{},
			},
		},
	}

	for _, test := range tests {
		moves := Transform(test.dels, 1<<test.forestRows, test.forestRows)
		fmt.Println(moves)

		for i := range moves {
			for j := range moves[i] {
				if moves[i][j].to != test.expected[i][j].to {
					t.Errorf("TestTransform fail: expected %v but got %v",
						test.expected, moves)
				}

				if moves[i][j].from != test.expected[i][j].from {
					t.Errorf("TestTransform fail: expected %v but got %v",
						test.expected, moves)
				}
			}
		}
	}
}

func TestDeTwin(t *testing.T) {
	var tests = []struct {
		forestRows uint8
		before     []uint64
		expected   []uint64
	}{
		{3, []uint64{0, 1, 9}, []uint64{12}},
		{3, []uint64{0, 1, 4, 9, 11}, []uint64{4, 11, 12}},
		{4, []uint64{00, 01, 04, 06, 10, 11, 17, 20}, []uint64{04, 06, 24, 26}},
		{4, []uint64{8, 9, 10, 24, 25}, []uint64{10, 20, 28}},
		{4, []uint64{00, 02, 18, 19, 21, 27}, []uint64{00, 02, 21, 25, 27}},
	}

	for _, test := range tests {
		deTwin(&test.before, test.forestRows)

		if len(test.before) != len(test.expected) {
			t.Errorf("TestDeTwin Error: expected %d but got %d",
				len(test.expected), len(test.before))
		}

		for i := range test.before {
			if test.before[i] != test.expected[i] {
				t.Errorf("TestDeTwin Error: expected %v but got %v",
					test.expected, test.before)
			}
		}
	}
}

func TestDeTwinRand(t *testing.T) {
	rand.Seed(time.Now().Unix())

	for x := 0; x < 10; x++ {
		// Forest with at least 3 rows but less than 11 rows.
		forestRows := uint8(rand.Intn(11-3) + 3)

		// Maximum number of leaves the accumulator can have.
		numLeaves := 1 << forestRows
		delAmount := 10
		if numLeaves < 10 {
			delAmount = rand.Intn(numLeaves)
		}

		// Generate the dels randomly.
		dels := make([]uint64, 0, delAmount)
		for i := 0; i < delAmount; i++ {
			randNum := uint64(rand.Intn(numLeaves))
			for slices.Contains(dels, randNum) {
				randNum++
			}

			dels = append(dels, randNum)
		}

		slices.Sort(dels)

		fmt.Println("before: ", dels, forestRows)
		deTwin(&dels, forestRows)
		fmt.Println("after : ", dels, forestRows)

		fmt.Println()
		fmt.Println()
		fmt.Println()

		// Check that there are no siblings in the del slice.
		for i := 0; i < len(dels); i++ {
			if i+1 < len(dels) && isNextElemSibling(dels, i) {
				err := fmt.Errorf("DeTwin error: dels[i]:%d and dels[i+1]:%d are siblings",
					dels[i], dels[i+1])
				t.Fatal(err)
			}
		}
	}
}
