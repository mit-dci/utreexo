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
		numLeaves  uint64
		dels       []uint64
		expected   [][]arrow
	}{
		// 14
		// |---------------\
		// 12              13
		// |-------\       |-------\
		// 08      09      10      11
		// |---\   |---\   |---\   |---\
		// 00* 01* 02* 03* 04* 05* 06* 07*
		{
			3,
			8,
			[]uint64{0, 1, 2, 3, 4, 5, 6, 7},
			[][]arrow{
				{},
				{},
				{},
				{{from: 14, to: 14}},
			},
		},

		// 14
		// |---------------\
		// 12              13
		// |-------\       |-------\
		// 08      09      10      11
		// |---\   |---\   |---\   |---\
		// 00* 01* 02  03  04* 05  06  07
		{
			3,
			8,
			[]uint64{0, 1, 4},
			[][]arrow{
				{{from: 5, to: 10}},
				{{from: 9, to: 12}},
				{},
				{},
			},
		},

		// 14
		// |---------------\
		// 12              13
		// |-------\       |-------\
		// --      --      10      11
		// |---\   |---\   |---\   |---\
		// --  --  --  --  --  --  06  07
		{
			3,
			8,
			[]uint64{6, 12},
			[][]arrow{
				{{from: 7, to: 11}},
				{},
				{{from: 13, to: 14}},
				{},
			},
		},

		// 14
		// |---------------\
		// 12              13
		// |-------\       |-------\
		// 08      09      10      11
		// |---\   |---\   |---\   |---\
		// 00* 01* 02* 03* 04* 05  06* 07*
		{
			3,
			8,
			[]uint64{0, 1, 2, 3, 4, 6, 7},
			[][]arrow{
				{},
				{},
				{{from: 5, to: 14}},
				{},
			},
		},

		// 14
		// |---------------\
		// 12              13
		// |-------\       |-------\
		// 08      09*     10      11*
		// |---\   |---\   |---\   |---\
		// 00* 01* --  --  04* 05  06  07
		{
			3,
			8,
			[]uint64{0, 1, 4, 9, 11},
			[][]arrow{
				{},
				{},
				{{from: 5, to: 14}},
				{},
			},
		},

		// 14
		// |---------------\
		// 12*             13
		// |-------\       |-------\
		// --      --      10      11
		// |---\   |---\   |---\   |---\
		// --  --  --  --  04  05  06  07
		{
			3,
			8,
			[]uint64{12},
			[][]arrow{
				{},
				{},
				{{from: 13, to: 14}},
				{},
			},
		},

		// 14
		// |---------------\
		// 12              13
		// |-------\       |-------\
		// 08      09      10      11*
		// |---\   |---\   |---\   |---\
		// 00* 01* 02* 03* 04* 05  --  --
		{
			3,
			8,
			[]uint64{0, 1, 2, 3, 4, 11},
			[][]arrow{
				{},
				{},
				{{from: 5, to: 14}},
				{},
			},
		},

		// 30
		// |-------------------------------\
		// 28                              29
		// |---------------\               |---------------\
		// 24*             25*             26              27
		// |-------\       |-------\       |-------\       |-------\
		// --      --      --      --      20      21      22      23
		// |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\
		// --  --  --  --  --  --  --  --  08* 09* 10* 11  12  13  14  15
		{
			4,
			16,
			[]uint64{8, 9, 10, 24, 25},
			[][]arrow{
				{},
				{{from: 11, to: 26}},
				{},
				{{from: 29, to: 30}},
				{},
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
		// 16      17      18*     19*     20      21*     --      --
		// |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\
		// 00* 01  02* 03  --  --  --  --  08  09  --  --  --  --  --  --
		{
			4,
			16,
			[]uint64{0, 2, 18, 19, 21, 27},
			[][]arrow{
				{{from: 01, to: 16}, {from: 3, to: 17}},
				{},
				{{from: 24, to: 28}, {from: 20, to: 29}},
				{},
				{},
			},
		},
	}

	for _, test := range tests {
		moves := Transform(test.dels, test.numLeaves, test.forestRows)
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

func TestCalcDirtyNodes(t *testing.T) {
	var tests = []struct {
		forestRows uint8
		numLeaves  uint64
		moves      [][]arrow
		expected   [][]uint64
	}{

		// em
		// |---------------\
		// 12              em
		// |-------\       |-------\
		// 08      09      10      em
		// |---\   |---\   |---\   |---\
		// 00* 01  02  03  04* 05* em  em
		{
			3,
			6,
			[][]arrow{
				{{from: 1, to: 8}},
				{},
				{},
				{},
			},
			[][]uint64{
				{},
				{},
				{12},
				{},
			},
		},

		// 14
		// |---------------\
		// 12              13
		// |-------\       |-------\
		// 08      09      10      11
		// |---\   |---\   |---\   |---\
		// 00* 01* 02  03  04* 05  06  07
		{
			3,
			8,
			[][]arrow{
				{{from: 5, to: 10}},
				{{from: 9, to: 12}},
				{},
				{},
			},
			[][]uint64{
				{},
				{},
				{13},
				{14},
			},
		},

		// 14
		// |---------------\
		// 12              13
		// |-------\       |-------\
		// 08      09      10      11
		// |---\   |---\   |---\   |---\
		// 00* 01* 02* 03* 04* 05  06* 07*
		{
			3,
			8,
			[][]arrow{
				{},
				{},
				{{from: 5, to: 14}},
				{},
			},
			[][]uint64{
				{},
				{},
				{},
				{},
			},
		},

		// 14
		// |---------------\
		// 12*             13
		// |-------\       |-------\
		// --      --      10      11
		// |---\   |---\   |---\   |---\
		// --  --  --  --  04* 05  06* 07
		{
			3,
			8,
			[][]arrow{
				{{from: 5, to: 10}, {from: 7, to: 11}},
				{},
				{{from: 13, to: 14}},
				{},
			},
			[][]uint64{
				{},
				{},
				{},
				{14},
			},
		},

		// em
		// |---------------\
		// 12              em
		// |-------\       |-------\
		// 08      09      10      em
		// |---\   |---\   |---\   |---\
		// 00* 01* 02  03  04* 05  06* em
		{
			3,
			7,
			[][]arrow{
				{{from: 5, to: 10}},
				{{from: 9, to: 12}},
				{},
				{},
			},
			[][]uint64{
				{},
				{},
				{},
				{},
			},
		},

		// 14
		// |---------------\
		// 12*             13
		// |-------\       |-------\
		// --      --      10      11
		// |---\   |---\   |---\   |---\
		// --  --  --  --  --  --  06* 07
		{
			3,
			8,
			[][]arrow{
				{{from: 7, to: 11}},
				{},
				{{from: 13, to: 14}},
				{},
			},
			[][]uint64{
				{},
				{},
				{},
				{14},
			},
		},

		// 30
		// |-------------------------------\
		// 28                              29
		// |---------------\               |---------------\
		// 24*             25              26              27
		// |-------\       |-------\       |-------\       |-------\
		// --      --      18      19      20      21      22      23
		// |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\
		// --  --  --  --  --  --  06* 07  08  09  10  11  12  13  14  15
		{
			4,
			16,
			[][]arrow{
				{{from: 7, to: 19}},
				{},
				{{from: 25, to: 28}},
				{},
				{},
			},
			[][]uint64{
				{},
				{},
				{},
				{28},
				{30},
			},
		},

		// em
		// |-------------------------------\
		// 28                              em
		// |---------------\               |---------------\
		// 24              25              26              em
		// |-------\       |-------\       |-------\       |-------\
		// 16*     17      18*     19      20      21      em      em
		// |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\
		// --  --  --  --  --  --  06* 07* 08* 09  10  11  em  em  em  em
		{
			4,
			12,
			[][]arrow{
				{{from: 9, to: 20}},
				{},
				{{from: 17, to: 28}},
				{},
				{},
			},
			[][]uint64{
				{},
				{},
				{26},
				{},
				{},
			},
		},

		// 62
		// |---------------------------------------------------------------\
		// 60                                                              61
		// |-------------------------------\                               |-------------------------------\
		// 56*                             57                              58                              59
		// |---------------\               |---------------\               |---------------\               |---------------\
		//                                 50              51              52              53              54              55
		// |-------\       |-------\       |-------\       |-------\       |-------\       |-------\       |-------\       |-------\
		//                                 36      37      38      39      40      41      42      43      44      45      46      47
		// |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\
		//                                 08* 09  10  11  12  13  14  15  16* 17  18  19  20  21  22  23  24  25  26  27  28  29  30  31
		{
			5,
			32,
			[][]arrow{
				{{from: 9, to: 36}, {from: 17, to: 40}},
				{},
				{},
				{{from: 57, to: 60}},
				{},
				{},
			},
			[][]uint64{
				{},
				{},
				{52},
				{56},
				{},
				{62},
			},
		},

		// 62
		// |---------------------------------------------------------------\
		// 60*                                                             61
		// |-------------------------------\                               |-------------------------------\
		//                                                                 58                              59#
		// |---------------\               |---------------\               |---------------\               |---------------\
		//                                                                 52              53              54#             55
		// |-------\       |-------\       |-------\       |-------\       |-------\       |-------\       |-------\       |-------\
		//                                                                 40      41      42      43      44      45      46      47
		// |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\
		//                                                                 16  17  18  19  20  21  22  23  24* 25  26  27  28  29  30* 31*
		{
			5,
			32,
			[][]arrow{
				{{from: 25, to: 44}},
				{{from: 46, to: 55}},
				{},
				{},
				{{from: 61, to: 62}},
				{},
			},
			[][]uint64{
				{},
				{},
				{},
				{58},
				{61},
				{},
			},
		},

		// 62
		// |---------------------------------------------------------------\
		// 60*                                                             61
		// |-------------------------------\                               |-------------------------------\
		//                                                                 58                              59
		// |---------------\               |---------------\               |---------------\               |---------------\
		//                                                                 52              53              54              55
		// |-------\       |-------\       |-------\       |-------\       |-------\       |-------\       |-------\       |-------\
		//                                                                 40      41      42      43      44      45      46      47
		// |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\
		//                                                                                         22* 23
		// To find out where 53 is going.
		// 48, 49, 50, 51, | 52, 53, 54, 55 <- right (but trash this because we're going up one)
		// 52, 53, | 54, 55 <- left
		// 52, | 53 <- right
		// Those are the paths to take to get to the new position from the root. Just remove from the front by as many rows we're going up.
		//
		// For 42 rise 1:
		// 32, 33, 34, 35, 36, 37, 38, 39, | 40, 41, 42, 43, 44, 45, 46, 47 <-right (but trash this because we're going up one)
		// 40, 41, 42, 43, | 44, 45, 46, 47 <-left
		// 40, 41, | 42, 43 <-right
		// 42, | 43 <-left
		// For rise 1, 42 goes to 50
		//
		// Ok I think it matters more if how much older your descendent that's moving is. Your parent moving is easy. Your grandparent moving
		// is harder.
		//
		// 42 bitfields:
		// 42, | 43 <-left
		// 40, 41, | 42, 43 <-right
		// 40, 41, 42, 43, | 44, 45, 46, 47 <-left
		// 32, 33, 34, 35, 36, 37, 38, 39, | 40, 41, 42, 43, 44, 45, 46, 47 <-right
		//
		// For 42:
		// 43 del:
		// 42, | 43 <-left (trash)
		// 40, 41, | 42, 43 <-right
		// 40, 41, 42, 43, | 44, 45, 46, 47 <-left
		// 32, 33, 34, 35, 36, 37, 38, 39, | 40, 41, 42, 43, 44, 45, 46, 47 <-right
		//
		// 52 del:
		// 42, | 43 <-left
		// 40, 41, | 42, 43 <-right (trash)
		// 40, 41, 42, 43, | 44, 45, 46, 47 <-left
		// 32, 33, 34, 35, 36, 37, 38, 39, | 40, 41, 42, 43, 44, 45, 46, 47 <-right
		//
		// 59 del:
		// 42, | 43 <-left
		// 40, 41, | 42, 43 <-right
		// 40, 41, 42, 43, | 44, 45, 46, 47 <-left (trash)
		// 32, 33, 34, 35, 36, 37, 38, 39, | 40, 41, 42, 43, 44, 45, 46, 47 <-right
		//
		// 60 del:
		// 42, | 43 <-left
		// 40, 41, | 42, 43 <-right
		// 40, 41, 42, 43, | 44, 45, 46, 47 <-left
		// 32, 33, 34, 35, 36, 37, 38, 39, | 40, 41, 42, 43, 44, 45, 46, 47 <-right (trash)
		{
			5,
			32,
			[][]arrow{
				{},
				{{from: 23, to: 43}},
				{},
				{},
				{{from: 61, to: 62}},
				{},
			},
			[][]uint64{
				{},
				{},
				{},
				{57},
				{},
				{},
			},
		},
	}

	for i, test := range tests {
		dirtyRows := calcDirtyNodes2(test.moves, test.numLeaves, test.forestRows)

		fmt.Println("dirty Nodes", dirtyRows, i)

		if len(dirtyRows) != len(test.expected) {
			t.Fatalf("TestCalcDirtyNodes fail %d: expected %d rows, got %d",
				i, len(test.expected), len(dirtyRows))
		}

		for row, dirtyRow := range dirtyRows {
			if len(dirtyRow) != len(test.expected[row]) {
				t.Fatalf("TestCalcDirtyNodes fail %d: at row %d, expected %d nodes but got %d",
					i, row, len(test.expected[row]), len(dirtyRow))
			}

			for i, dirty := range dirtyRow {
				expectedDirty := test.expected[row][i]

				if dirty != expectedDirty {
					t.Errorf("TestCalcDirtyNodes fail: expected %d but got %d",
						expectedDirty, dirty)
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
		{3, []uint64{0, 1, 2, 3, 4, 11}, []uint64{4, 11, 12}},
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
