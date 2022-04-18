package accumulator

import (
	"fmt"
	"math/rand"
	"strconv"
	"testing"
	"time"

	"golang.org/x/exp/slices"
)

func TestInsertSort(t *testing.T) {
	dels := []uint64{1, 2, 2, 3}
	insertSort(&dels, 1)

	fmt.Println(dels)
	dels = removeDuplicateInt(dels)
	fmt.Println(dels)
}

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
			//	{{from: 11, to: 21}},
			//	{{from: 21, to: 26}},
			//	{},
			//	{{from: 29, to: 30}},
			//	{},
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

		// 62
		// |---------------------------------------------------------------\
		// 60                                                              61
		// |-------------------------------\                               |-------------------------------\
		//                                                                 58                              59
		// |---------------\               |---------------\               |---------------\               |---------------\
		//                                                                 52              53              54              55
		// |-------\       |-------\       |-------\       |-------\       |-------\       |-------\       |-------\       |-------\
		//                                                                 40      41      42      43      44      45      46      47
		// |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\
		//                                                                 16  17* 18* 19* 20  21  22  23  24  25  26  27  28  29  30  31
		{
			5,
			32,
			[]uint64{17, 18, 19},
			[][]arrow{
				{},
				{{from: 16, to: 52}},
				{},
				{},
				{},
				{},
			},
		},

		// 126
		// |-------------------------------------------------------------------------------------------------------------------------------\
		// 123                                                                                                                             125
		// |---------------------------------------------------------------\                                                               |---------------------------------------------------------------\
		// 120                                                             121*                                                            122                                                             123
		// |-------------------------------\                               |-------------------------------\                               |-------------------------------\                               |-------------------------------\
		// 112                             113*                                                                                            116                             117                             118                             119
		// |---------------\               |---------------\               |---------------\               |---------------\               |---------------\               |---------------\               |---------------\               |---------------\
		// 96              97                                                                                                              104             105             106             107             108             109             110             111
		// |-------\       |-------\       |-------\       |-------\       |-------\       |-------\       |-------\       |-------\       |-------\       |-------\       |-------\       |-------\       |-------\       |-------\       |-------\       |-------\
		// 64      65      66      67                                                                                                      80      81      82      83      84      85      86      87      88      89      90      91      92      93      94      95
		// |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\
		//         02  03* 04* 05*                                                                                                         32  33  34  35  36  37  38  39  40  41  42  43  44  45  46  47  48  49  50  51  52  53  54  55  56  57  58  59  60  61  62  63
		{
			6,
			64,
			[]uint64{3, 4, 5, 113, 121},
			[][]arrow{
				{{from: 2, to: 65}},
				{{from: 67, to: 97}},
				{},
				{},
				{{from: 112, to: 124}},
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
		// 60                                                              61
		// |-------------------------------\                               |-------------------------------\
		//                                                                 58                              59
		// |---------------\               |---------------\               |---------------\               |---------------\
		//                                                                 52              53              54              55
		// |-------\       |-------\       |-------\       |-------\       |-------\       |-------\       |-------\       |-------\
		//                                                                 40      41      42      43      44      45      46      47
		// |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\
		//                                                                 16  17* 18* 19* 20  21  22  23  24  25  26  27  28  29  30  31
		{
			5,
			32,
			[][]arrow{
				{},
				{{from: 16, to: 52}},
				{},
				{},
				{},
				{},
			},
			[][]uint64{
				{},
				{},
				{},
				{58},
				{},
				{},
			},
		},
		// 126
		// |-------------------------------------------------------------------------------------------------------------------------------\
		// 124                                                                                                                             125
		// |---------------------------------------------------------------\                                                               |---------------------------------------------------------------\
		// 120                                                             121*                                                            122                                                             123
		// |-------------------------------\                               |-------------------------------\                               |-------------------------------\                               |-------------------------------\
		// 112                             113*                                                                                            116                             117                             118                             119
		// |---------------\               |---------------\               |---------------\               |---------------\               |---------------\               |---------------\               |---------------\               |---------------\
		// 96              97                                                                                                              104             105             106             107             108             109             110             111
		// |-------\       |-------\       |-------\       |-------\       |-------\       |-------\       |-------\       |-------\       |-------\       |-------\       |-------\       |-------\       |-------\       |-------\       |-------\       |-------\
		// 64      65      66      67                                                                                                      80      81      82      83      84      85      86      87      88      89      90      91      92      93      94      95
		// |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\
		//         02  03* 04* 05*                                                                                                         32  33  34  35  36  37  38  39  40  41  42  43  44  45  46  47  48  49  50  51  52  53  54  55  56  57  58  59  60  61  62  63
		{
			6,
			64,
			[][]arrow{
				{{from: 2, to: 65}},
				{{from: 67, to: 97}},
				{},
				{},
				{{from: 112, to: 124}},
				{},
				{},
			},
			[][]uint64{
				{},
				{},
				{},
				{},
				{120},
				{124},
				{126},
			},
		},

		// 62
		// |---------------------------------------------------------------\
		// 60                                                              61
		// |-------------------------------\                               |-------------------------------\
		// 56                              57                              58                              59
		// |---------------\               |---------------\               |---------------\               |---------------\
		// 48              49              50              51              52              53              54              55
		// |-------\       |-------\       |-------\       |-------\       |-------\       |-------\       |-------\       |-------\
		// 32      33      34      35      36      37      38      39      40      41      42      43      44      45      46      47
		// |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\
		// 00  01  02  03  04  05  06  07  08  09  10  11  12  13  14  15  16  17  18  19  20  21  22  23  24  25  26  27  28  29  30  31

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
		// 50 bitfields:
		// 110 010
		//
		// 22 bitfields:
		// 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, | 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31 <- right
		// 16, 17, 18, 19, 20, 21, 22, 23, | 24, 25, 26, 27, 28, 29, 30, 31 <- left
		// 16, 17, 18, 19, | 20, 21, 22, 23 <- right
		// 20, 21, | 22, 23 <- right
		// 22, | 23 <- left
		//
		// [right, left, right, right, left] <- full
		// [right, left, right, right, ----] <- row 0 del
		// [right, left, right, -----, left] <- row 1 del
		// [right, left, -----, right, left] <- row 2 del
		// [right, ----, right, right, left] <- row 3 del
		// [-----, left, right, right, left] <- row 4 del

		// 10110 <- 22
		// 101011
		//
		// 111110 <- 62
		// 111101 <- 61
		// 111010 <- 58
		//
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

func TestCalcNextPosition(t *testing.T) {
	var tests = []struct {
		position, numLeaves    uint64
		deletionRow, forestRow uint8
		expected               uint64
	}{
		{position: 22, numLeaves: 32, deletionRow: 0, forestRow: 5, expected: 43},
		{position: 22, numLeaves: 32, deletionRow: 1, forestRow: 5, expected: 42},
		{position: 22, numLeaves: 32, deletionRow: 2, forestRow: 5, expected: 42},
		{position: 22, numLeaves: 32, deletionRow: 3, forestRow: 5, expected: 46},
		{position: 22, numLeaves: 32, deletionRow: 4, forestRow: 5, expected: 38},

		{position: 50, numLeaves: 32, deletionRow: 3, forestRow: 5, expected: 56},

		{position: 12, numLeaves: 15, deletionRow: 0, forestRow: 4, expected: 22},
		{position: 8, numLeaves: 15, deletionRow: 1, forestRow: 4, expected: 20},
		{position: 2, numLeaves: 15, deletionRow: 2, forestRow: 4, expected: 18},
		{position: 16, numLeaves: 15, deletionRow: 2, forestRow: 4, expected: 24},

		{position: 112, numLeaves: 64, deletionRow: 3, forestRow: 6, expected: 120},
		{position: 120, numLeaves: 64, deletionRow: 4, forestRow: 6, expected: 124},
	}

	for i, test := range tests {
		gotPos := calcNextPosition(test.position, test.numLeaves, test.deletionRow, test.forestRow)
		if gotPos != test.expected {
			t.Errorf("TestCalcNextPosition %d fail: expected %d, got %d\n",
				i, test.expected, gotPos)
		}

		if i == 1 {
			calcNextPosition2(test.position, test.numLeaves, test.deletionRow, test.forestRow)
		}
	}
}

func TestCalcPos2(t *testing.T) {
	forestRows := uint8(3)
	calcNextPosition2(3, 8, 1, forestRows)

	mask := uint64(2<<forestRows) - 1
	fmt.Println(strconv.FormatUint(mask, 2))
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

func isNextElemSibling(dels []uint64, idx int) bool {
	return dels[idx]|1 == dels[idx+1]
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
