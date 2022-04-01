package accumulator

import (
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"sort"
	"testing"
	"testing/quick"
)

func TestForestSwaplessSimple(t *testing.T) {
	var tests = []struct {
		startLeaves []Leaf
		dels        []uint64
		expected    map[uint64]Hash
	}{
		// 14
		// |---------------\
		// 12              13
		// |-------\       |-------\
		// 08      09      10      11
		// |---\   |---\   |---\   |---\
		// 00* 01* 02* 03* 04* 05  06* 07*
		{
			[]Leaf{
				{Hash: Hash{1}},
				{Hash: Hash{2}},
				{Hash: Hash{3}},
				{Hash: Hash{4}},
				{Hash: Hash{5}},
				{Hash: Hash{6}},
				{Hash: Hash{7}},
				{Hash: Hash{8}},
			},
			[]uint64{0, 1, 2, 3, 4, 6, 7},
			map[uint64]Hash{
				0:  empty,
				1:  empty,
				2:  empty,
				3:  empty,
				4:  empty,
				5:  empty,
				6:  empty,
				7:  empty,
				8:  empty,
				9:  empty,
				10: empty,
				11: empty,
				12: empty,
				13: empty,
				14: Hash{6},
			},
		},

		// 14
		// |---------------\
		// 12              13
		// |-------\       |-------\
		// 08      09      10      11
		// |---\   |---\   |---\   |---\
		// 00* 01* 02* 03* 04  05  06  07
		{
			[]Leaf{
				{Hash: Hash{1}},
				{Hash: Hash{2}},
				{Hash: Hash{3}},
				{Hash: Hash{4}},
				{Hash: Hash{5}},
				{Hash: Hash{6}},
				{Hash: Hash{7}},
				{Hash: Hash{8}},
			},
			[]uint64{0, 1, 2, 3},
			map[uint64]Hash{
				0:  empty,
				1:  empty,
				2:  empty,
				3:  empty,
				4:  empty,
				5:  empty,
				6:  empty,
				7:  empty,
				8:  Hash{5},
				9:  Hash{6},
				10: Hash{7},
				11: Hash{8},
				12: parentHash(Hash{5}, Hash{6}),
				13: parentHash(Hash{7}, Hash{8}),
				14: parentHash(
					parentHash(Hash{5}, Hash{6}),
					parentHash(Hash{7}, Hash{8})),
			},
		},

		// 14
		// |---------------\
		// 12              13
		// |-------\       |-------\
		// 08      09      10      11
		// |---\   |---\   |---\   |---\
		// 00* 01* 02  03* 04  05  06  07
		{
			[]Leaf{
				{Hash: Hash{1}},
				{Hash: Hash{2}},
				{Hash: Hash{3}},
				{Hash: Hash{4}},
				{Hash: Hash{5}},
				{Hash: Hash{6}},
				{Hash: Hash{7}},
				{Hash: Hash{8}},
			},
			[]uint64{0, 1, 3},
			map[uint64]Hash{
				0:  empty,
				1:  empty,
				2:  empty,
				3:  empty,
				4:  Hash{5},
				5:  Hash{6},
				6:  Hash{7},
				7:  Hash{8},
				8:  empty,
				9:  empty,
				10: parentHash(Hash{5}, Hash{6}),
				11: parentHash(Hash{7}, Hash{8}),
				12: Hash{3},
				13: parentHash(
					parentHash(Hash{5}, Hash{6}),
					parentHash(Hash{7}, Hash{8})),
				14: parentHash(
					Hash{3},
					parentHash(parentHash(Hash{5}, Hash{6}),
						parentHash(Hash{7}, Hash{8})),
				),
			},
		},

		{
			[]Leaf{
				{Hash: Hash{1}},
				{Hash: Hash{2}},
				{Hash: Hash{3}},
				{Hash: Hash{4}},
				{Hash: Hash{5}},
				{Hash: Hash{6}},
				{Hash: Hash{7}},
				{Hash: Hash{8}},
				{Hash: Hash{9}},
			},
			[]uint64{0, 1, 2},
			map[uint64]Hash{
				0:  empty,
				1:  empty,
				2:  empty,
				3:  empty,
				4:  Hash{5},
				5:  Hash{6},
				6:  Hash{7},
				7:  Hash{8},
				8:  Hash{9},
				16: empty,
				17: empty,
				18: parentHash(Hash{5}, Hash{6}),
				19: parentHash(Hash{7}, Hash{8}),
				25: parentHash(
					parentHash(Hash{5}, Hash{6}),
					parentHash(Hash{7}, Hash{8})),
				28: parentHash(
					Hash{4},
					parentHash(
						parentHash(Hash{5}, Hash{6}),
						parentHash(Hash{7}, Hash{8})),
				),
			},
		},
	}

	for i, test := range tests {
		//if i != 1 {
		//	continue
		//}

		f := NewForest(RamForest, nil, "", 0)

		_, err := f.Modify(test.startLeaves, nil)
		if err != nil {
			t.Fatal(err)
		}

		fmt.Println(f.ToString())

		err = f.removeSwapless(test.dels)
		if err != nil {
			t.Fatal(err)
		}

		fmt.Println(f.ToString())

		for pos, expectedHash := range test.expected {
			hash := f.data.read(pos)

			if hash != expectedHash {
				t.Errorf("TestForestSwaplessSimple Fail: test %d failed "+
					"for position %d, expected %s got %s",
					i, pos, hex.EncodeToString(expectedHash[:]),
					hex.EncodeToString(hash[:]))
			}
		}
	}
}

type modification struct {
	add []Leaf
	del []uint64

	expected      map[MiniHash]uint64
	expectedEmpty []uint64
	expectedRoots []Hash
}

func stringToHash(str string) Hash {
	hash, err := hex.DecodeString(str)
	if err != nil {
		// Ok to panic since this function is only used for testing.
		// If it panics, it means that the hardcoded string is wrong.
		panic(err)
	}

	return *(*Hash)(hash)
}

func TestSwaplessModify(t *testing.T) {
	var tests = []struct {
		name     string
		modifies []modification
	}{
		{
			"Only adds",
			[]modification{
				// Only adds block 1
				{
					add: []Leaf{
						{Hash: Hash{1}},
						{Hash: Hash{2}},
						{Hash: Hash{3}},
						{Hash: Hash{4}},
						{Hash: Hash{5}},
						{Hash: Hash{6}},
						{Hash: Hash{7}},
						{Hash: Hash{8}},
						{Hash: Hash{9}},
					},
					del: nil,
					expected: map[MiniHash]uint64{
						Hash{1}.Mini(): 0,
						Hash{2}.Mini(): 1,
						Hash{3}.Mini(): 2,
						Hash{4}.Mini(): 3,
						Hash{5}.Mini(): 4,
						Hash{6}.Mini(): 5,
						Hash{7}.Mini(): 6,
						Hash{8}.Mini(): 7,
						Hash{9}.Mini(): 8,
					},
					expectedEmpty: nil,
					expectedRoots: []Hash{
						stringToHash("ee4c7313527e3ee54ee97793cd35e8df" +
							"4f7fcf3b0012bec7e7cfdb9ace0cd3fd"),
						stringToHash("09000000000000000000000000000000" +
							"00000000000000000000000000000000"),
					},
				},

				// Only adds block 2
				{
					add: []Leaf{
						{Hash: Hash{10}},
						{Hash: Hash{11}},
						{Hash: Hash{12}},
						{Hash: Hash{13}},
						{Hash: Hash{14}},
						{Hash: Hash{15}},
						{Hash: Hash{16}},
						{Hash: Hash{17}},
						{Hash: Hash{18}},
						{Hash: Hash{19}},
						{Hash: Hash{20}},
					},
					del: nil,
					expected: map[MiniHash]uint64{
						Hash{1}.Mini():  0,
						Hash{2}.Mini():  1,
						Hash{3}.Mini():  2,
						Hash{4}.Mini():  3,
						Hash{5}.Mini():  4,
						Hash{6}.Mini():  5,
						Hash{7}.Mini():  6,
						Hash{8}.Mini():  7,
						Hash{9}.Mini():  8,
						Hash{10}.Mini(): 9,
						Hash{11}.Mini(): 10,
						Hash{12}.Mini(): 11,
						Hash{13}.Mini(): 12,
						Hash{14}.Mini(): 13,
						Hash{15}.Mini(): 14,
						Hash{16}.Mini(): 15,
						Hash{17}.Mini(): 16,
						Hash{18}.Mini(): 17,
						Hash{19}.Mini(): 18,
						Hash{20}.Mini(): 19,
					},
					expectedEmpty: nil,
					expectedRoots: []Hash{
						stringToHash("2c1ecb81b164c6dff4a6d89c19fcddf1" +
							"5356f37e5a1f5e82f505c5b9ef856e25"),
						stringToHash("8b34c7baf39f2216fd352a86616adaa5" +
							"1f394103b0524d9dc2430045de50d116"),
					},
				},
			},
		},

		{
			"Delete once",
			[]modification{
				// Delete once block 1
				{
					add: []Leaf{
						{Hash: Hash{1}},
						{Hash: Hash{2}},
						{Hash: Hash{3}},
						{Hash: Hash{4}},
						{Hash: Hash{5}},
						{Hash: Hash{6}},
						{Hash: Hash{7}},
						{Hash: Hash{8}},
						{Hash: Hash{9}},
						{Hash: Hash{10}},
						{Hash: Hash{11}},
						{Hash: Hash{12}},
						{Hash: Hash{13}},
						{Hash: Hash{14}},
						{Hash: Hash{15}},
						{Hash: Hash{16}},
					},
					del: nil,
					expected: map[MiniHash]uint64{
						Hash{1}.Mini():  0,
						Hash{2}.Mini():  1,
						Hash{3}.Mini():  2,
						Hash{4}.Mini():  3,
						Hash{5}.Mini():  4,
						Hash{6}.Mini():  5,
						Hash{7}.Mini():  6,
						Hash{8}.Mini():  7,
						Hash{9}.Mini():  8,
						Hash{10}.Mini(): 9,
						Hash{11}.Mini(): 10,
						Hash{12}.Mini(): 11,
						Hash{13}.Mini(): 12,
						Hash{14}.Mini(): 13,
						Hash{15}.Mini(): 14,
						Hash{16}.Mini(): 15,
					},
					expectedEmpty: nil,
					expectedRoots: []Hash{
						stringToHash("2c1ecb81b164c6dff4a6d89c19fcddf1" +
							"5356f37e5a1f5e82f505c5b9ef856e25"),
					},
				},

				// Delete once block 2
				{
					add: nil,
					del: []uint64{0, 2, 3, 4, 5, 6, 7},
					expected: map[MiniHash]uint64{
						Hash{2}.Mini():  28,
						Hash{9}.Mini():  8,
						Hash{10}.Mini(): 9,
						Hash{11}.Mini(): 10,
						Hash{12}.Mini(): 11,
						Hash{13}.Mini(): 12,
						Hash{14}.Mini(): 13,
						Hash{15}.Mini(): 14,
						Hash{16}.Mini(): 15,
					},
					expectedEmpty: []uint64{0, 1, 2, 3, 4, 5, 6, 7, 16, 17,
						18, 19, 24, 25},
					expectedRoots: []Hash{
						stringToHash("e018100dadcc58df80b4e955fffcc3fd" +
							"a1b3c9831b86bb6b7a80f824c046c360"),
					},
				},
			},
		},

		{
			"edge case",
			[]modification{
				// 1st block.
				{
					add: []Leaf{
						{Hash: Hash{1}},
						{Hash: Hash{2}},
						{Hash: Hash{3}},
						{Hash: Hash{4}},
						{Hash: Hash{5}},
						{Hash: Hash{6}},
						{Hash: Hash{7}},
						{Hash: Hash{8}},
					},
					del: nil,
					expected: map[MiniHash]uint64{
						Hash{1}.Mini(): 0,
						Hash{2}.Mini(): 1,
						Hash{3}.Mini(): 2,
						Hash{4}.Mini(): 3,
						Hash{5}.Mini(): 4,
						Hash{6}.Mini(): 5,
						Hash{7}.Mini(): 6,
						Hash{8}.Mini(): 7,
					},
					expectedEmpty: nil,
					expectedRoots: []Hash{
						stringToHash("ee4c7313527e3ee54ee97793cd35e8df" +
							"4f7fcf3b0012bec7e7cfdb9ace0cd3fd"),
					},
				},

				{
					add:           nil,
					del:           []uint64{0, 1, 2, 4},
					expectedEmpty: []uint64{0, 1, 2, 3, 4, 5, 8, 9},
					expectedRoots: []Hash{
						stringToHash("d10790b861666df44c6dfb52aa700ae3" +
							"c469ebf4e06de7e2c55da7dae6fd93f0"),
					},
				},

				{
					add:           nil,
					del:           []uint64{6, 12},
					expectedEmpty: []uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11},
					expectedRoots: []Hash{
						stringToHash("17778a33dcae949b32cec24c8d06971f" +
							"b4802f0bf463ec279e6efa323a09fbd9"),
					},
				},
			},
		},

		{
			"edge case 2",
			[]modification{
				// 1st block.
				{
					add: []Leaf{
						{Hash: Hash{1}},
						{Hash: Hash{2}},
						{Hash: Hash{3}},
						{Hash: Hash{4}},
						{Hash: Hash{5}},
						{Hash: Hash{6}},
						{Hash: Hash{7}},
						{Hash: Hash{8}},
					},
					del: nil,
					expected: map[MiniHash]uint64{
						Hash{1}.Mini(): 0,
						Hash{2}.Mini(): 1,
						Hash{3}.Mini(): 2,
						Hash{4}.Mini(): 3,
						Hash{5}.Mini(): 4,
						Hash{6}.Mini(): 5,
						Hash{7}.Mini(): 6,
						Hash{8}.Mini(): 7,
					},
					expectedEmpty: nil,
					expectedRoots: []Hash{
						stringToHash("ee4c7313527e3ee54ee97793cd35e8df" +
							"4f7fcf3b0012bec7e7cfdb9ace0cd3fd"),
					},
				},

				{
					add:           nil,
					del:           []uint64{0, 1, 2, 4},
					expectedEmpty: []uint64{0, 1, 2, 3, 4, 5, 8, 9},
					expectedRoots: []Hash{
						stringToHash("d10790b861666df44c6dfb52aa700ae3" +
							"c469ebf4e06de7e2c55da7dae6fd93f0"),
					},
				},

				{
					add:           nil,
					del:           []uint64{6, 12},
					expectedEmpty: []uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11},
					expectedRoots: []Hash{
						stringToHash("17778a33dcae949b32cec24c8d06971f" +
							"b4802f0bf463ec279e6efa323a09fbd9"),
					},
				},
			},
		},

		{
			"test empty deletions",
			[]modification{
				// 1st block.
				{
					add: []Leaf{
						{Hash: Hash{1}},
						{Hash: Hash{2}},
						{Hash: Hash{3}},
						{Hash: Hash{4}},
						{Hash: Hash{5}},
						{Hash: Hash{6}},
						{Hash: Hash{7}},
						{Hash: Hash{8}},
						{Hash: Hash{9}},
						{Hash: Hash{10}},
						{Hash: Hash{11}},
						{Hash: Hash{12}},
						{Hash: Hash{13}},
						{Hash: Hash{14}},
						{Hash: Hash{15}},
						{Hash: Hash{16}},
					},
					del: nil,
					expected: map[MiniHash]uint64{
						Hash{1}.Mini():  0,
						Hash{2}.Mini():  1,
						Hash{3}.Mini():  2,
						Hash{4}.Mini():  3,
						Hash{5}.Mini():  4,
						Hash{6}.Mini():  5,
						Hash{7}.Mini():  6,
						Hash{8}.Mini():  7,
						Hash{9}.Mini():  8,
						Hash{10}.Mini(): 9,
						Hash{11}.Mini(): 10,
						Hash{12}.Mini(): 11,
						Hash{13}.Mini(): 12,
						Hash{14}.Mini(): 13,
						Hash{15}.Mini(): 14,
						Hash{16}.Mini(): 15,
					},
					expectedEmpty: nil,
					expectedRoots: []Hash{
						stringToHash("2c1ecb81b164c6dff4a6d89c19fcddf1" +
							"5356f37e5a1f5e82f505c5b9ef856e25"),
					},
				},

				{
					add: nil,
					del: []uint64{1, 2, 3, 4, 5, 6, 7, 8, 14},
					expectedEmpty: []uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 14, 15,
						16, 17, 18, 19, 24, 25},
					expectedRoots: []Hash{
						stringToHash("7ccdb5eb659438b7cd85f8c46788ecf0" +
							"ed07239f8c8bcdc63733917fd7b64c89"),
					},
				},

				{
					add: nil,
					del: []uint64{11, 12, 13, 28},
					expectedEmpty: []uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12,
						13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 26, 27},
					expectedRoots: []Hash{
						stringToHash("4b963007f029831e0759ed5bf0646b4d" +
							"77acc60e50aaec212b6add8e20bb0776"),
					},
				},
			},
		},

		{
			"4 blocks. Adds and dels",
			[]modification{
				// 1st block.
				{
					add: []Leaf{
						{Hash: Hash{1}},
						{Hash: Hash{2}},
						{Hash: Hash{3}},
						{Hash: Hash{4}},
						{Hash: Hash{5}},
						{Hash: Hash{6}},
						{Hash: Hash{7}},
						{Hash: Hash{8}},
						{Hash: Hash{9}},
					},
					del: nil,
					expected: map[MiniHash]uint64{
						Hash{1}.Mini(): 0,
						Hash{2}.Mini(): 1,
						Hash{3}.Mini(): 2,
						Hash{4}.Mini(): 3,
						Hash{5}.Mini(): 4,
						Hash{6}.Mini(): 5,
						Hash{7}.Mini(): 6,
						Hash{8}.Mini(): 7,
						Hash{9}.Mini(): 8,
					},
					expectedEmpty: nil,
					expectedRoots: []Hash{
						stringToHash("ee4c7313527e3ee54ee97793cd35e8df" +
							"4f7fcf3b0012bec7e7cfdb9ace0cd3fd"),
						stringToHash("09000000000000000000000000000000" +
							"00000000000000000000000000000000"),
					},
				},

				// 2nd block.
				{
					add: []Leaf{
						{Hash: Hash{10}},
						{Hash: Hash{11}},
						{Hash: Hash{12}},
					},
					expected: map[MiniHash]uint64{
						Hash{3}.Mini():  16,
						Hash{4}.Mini():  17,
						Hash{6}.Mini():  18,
						Hash{7}.Mini():  6,
						Hash{8}.Mini():  7,
						Hash{9}.Mini():  8,
						Hash{10}.Mini(): 9,
						Hash{11}.Mini(): 10,
						Hash{12}.Mini(): 11,
					},
					del:           []uint64{0, 1, 4},
					expectedEmpty: []uint64{0, 1, 2, 3, 4, 5},
					expectedRoots: []Hash{
						stringToHash("536cd74e712f63e10ecec98f084024ed" +
							"6ab16db91f24847206e08c9c2af7f339"),
						stringToHash("f7a8b895af8b0261718021e0cfcabd9e" +
							"0cf17bfd6c8e2755ee6461c3c85b986e"),
					},
				},

				// 3rd block.
				{
					add: []Leaf{
						{Hash: Hash{13}},
						{Hash: Hash{14}},
						{Hash: Hash{15}},
						{Hash: Hash{16}},
						{Hash: Hash{17}},
					},
					expected: map[MiniHash]uint64{
						Hash{4}.Mini():  56,
						Hash{9}.Mini():  8,
						Hash{10}.Mini(): 9,
						Hash{11}.Mini(): 10,
						Hash{12}.Mini(): 11,
						Hash{13}.Mini(): 12,
						Hash{14}.Mini(): 13,
						Hash{15}.Mini(): 14,
						Hash{16}.Mini(): 15,
						Hash{17}.Mini(): 16,
					},
					del: []uint64{6, 7, 16, 18},
					expectedEmpty: []uint64{0, 1, 2, 3, 4, 5, 6, 7,
						32, 33, 34, 35, 48, 49},
					expectedRoots: []Hash{
						stringToHash("ab567775cdbd9373c465cb68e79ac367" +
							"b0cbf385c19aacd2ae0787bad39b0957"),
						stringToHash("11000000000000000000000000000000" +
							"00000000000000000000000000000000"),
					},
				},

				// 4th block.
				{
					add: []Leaf{
						{Hash: Hash{18}},
						{Hash: Hash{19}},
					},
					expected: map[MiniHash]uint64{
						Hash{4}.Mini():  56,
						Hash{9}.Mini():  36,
						Hash{10}.Mini(): 37,
						Hash{11}.Mini(): 38,
						Hash{12}.Mini(): 39,
						Hash{17}.Mini(): 16,
						Hash{18}.Mini(): 17,
						Hash{19}.Mini(): 18,
					},
					del: []uint64{12, 13, 14, 15},
					expectedEmpty: []uint64{0, 1, 2, 3, 4, 5, 6, 7,
						12, 13, 14, 15, 32, 33, 34, 35, 48, 49},
					expectedRoots: []Hash{
						stringToHash("c0fce95da61de813a7f043e23ac13dd0" +
							"7c47f52b2565315d5a43d89d9bef0904"),
						stringToHash("a3cdd313912ce74d5335dbddee5b8682" +
							"567ad6ac6ca4d33ac8dc20ea864989a3"),
						stringToHash("13000000000000000000000000000000" +
							"00000000000000000000000000000000"),
					},
				},
			},
		},
	}

	for _, test := range tests {
		//if i != 2 {
		//	continue
		//}
		//if test.name != "test empty deletions" {
		//	continue
		//}

		forest := NewForest(RamForest, nil, "", 0)

		for blockHeight, modify := range test.modifies {
			// Perform the modify.
			_, err := forest.ModifySwapless(modify.add, modify.del)
			if err != nil {
				t.Fatalf("TestSwaplessModify test \"%s\" fail at block %d. Modify err:%v",
					test.name, blockHeight, err)
			}

			fmt.Println(forest.ToString())
			for _, root := range forest.GetRoots() {
				fmt.Println(hex.EncodeToString(root[:]))
			}
			fmt.Println(forest.positionMap)

			// Check that all the leaves in the map are there and that the
			// values are the same.
			for miniHash, position := range modify.expected {
				gotPosition, found := forest.positionMap[miniHash]
				if !found {
					t.Fatalf("TestSwaplessModify test \"%s\" fail at block %d. "+
						"Couldn't find expected minihash of %s",
						test.name, blockHeight, hex.EncodeToString(miniHash[:]))
				}

				if gotPosition != position {
					t.Fatalf("TestSwaplessModify test \"%s\" fail at block %d. "+
						"For minihash %s, expected %d but got %d",
						test.name, blockHeight, hex.EncodeToString(miniHash[:]),
						position, gotPosition)
				}
			}

			// Check that the length of the roots match up.
			roots := forest.GetRoots()
			if len(roots) != len(modify.expectedRoots) {
				t.Fatalf("TestSwaplessModify test \"%s\" fail at block %d. "+
					"Expected %d roots but got %d", test.name, blockHeight,
					len(modify.expectedRoots), len(roots))
			}

			// Check that the roots match up.
			for i, expectedRoot := range modify.expectedRoots {
				if expectedRoot != roots[i] {
					t.Fatalf("TestSwaplessModify test \"%s\" fail at block %d. "+
						"Root %d mismatch. Expected %s, got %s", test.name,
						blockHeight, i,
						hex.EncodeToString(expectedRoot[:]),
						hex.EncodeToString(roots[i][:]))
				}
			}

			// Check that the empty positions are indeed empty.
			for _, expectedEmpty := range modify.expectedEmpty {
				readHash := forest.data.read(expectedEmpty)

				if readHash != empty {
					t.Fatalf("TestSwaplessModify test \"%s\" fail at block %d. "+
						"Position %d was expected to be empty but read %s",
						test.name, blockHeight, expectedEmpty,
						hex.EncodeToString(readHash[:]))
				}
			}
		}
	}
}

func TestSwapLessAdd(t *testing.T) {
	f := NewForest(RamForest, nil, "", 0)
	leaves := []Leaf{
		{Hash: Hash{1}},
		{Hash: Hash{2}},
		{Hash: Hash{3}},
		{Hash: Hash{4}},
		{Hash: Hash{5}},
		{Hash: Hash{6}},
		{Hash: Hash{7}},
		{Hash: Hash{8}},
		{Hash: Hash{9}},
	}

	// remap to expand the forest if needed
	numdels, numadds := 0, len(leaves)
	delta := int64(numadds - numdels) // watch 32/64 bit
	for int64(f.numLeaves)+delta > int64(1<<f.rows) {
		// 1<<f.rows, f.numLeaves+delta)
		err := f.reMap(f.rows + 1)
		if err != nil {
			t.Fatal(err)
		}
	}
	f.addSwapless(leaves)

	fmt.Println(f.ToString())
	fmt.Println(f.positionMap)

	dels := []uint64{0, 1, 2}
	f.removeSwapless(dels)

	fmt.Println(f.ToString())
	fmt.Println(f.positionMap)

	f.addSwapless([]Leaf{{Hash: Hash{10}}})

	fmt.Println(f.ToString())
	fmt.Println(f.positionMap)

	f.removeSwapless([]uint64{8, 9})
	fmt.Println(f.ToString())
	fmt.Println(f.positionMap)

	adds := []Leaf{
		{Hash: Hash{11}},
	}

	f.addSwapless(adds)
	fmt.Println(f.ToString())
	fmt.Println(f.positionMap)

	adds = []Leaf{
		{Hash: Hash{12}},
	}

	f.addSwapless(adds)
	fmt.Println(f.ToString())
	fmt.Println(f.positionMap)

	adds = []Leaf{
		{Hash: Hash{13}},
		{Hash: Hash{14}},
		{Hash: Hash{15}},
		{Hash: Hash{16}},
	}
	f.addSwapless(adds)
	fmt.Println(f.ToString())
	fmt.Println(f.positionMap)

	f.removeSwapless([]uint64{12})
	fmt.Println(f.ToString())
	fmt.Println(f.positionMap)

	proofPositions := NewPositionList()
	defer proofPositions.Free()

	targetss := []uint64{4, 20, 24}
	fmt.Printf("targets %v, proofpos %v\n", targetss, proofPositions.list)
	//ProofPositionsSwapless(targetss, f.numLeaves, f.rows, &proofPositions.list)
	ProofPositions(targetss, f.numLeaves, f.rows, &proofPositions.list)

	fmt.Printf("targets %v, proofpos %v\n", targetss, proofPositions.list)
	fmt.Printf("numLeaves: %d, f.rows: %d\n", f.numLeaves, f.rows)
}

func TestDeleteReverseOrder(t *testing.T) {
	f := NewForest(RamForest, nil, "", 0)
	leaf1 := Leaf{Hash: Hash{1}}
	leaf2 := Leaf{Hash: Hash{2}}
	_, err := f.Modify([]Leaf{leaf1, leaf2}, nil)
	if err != nil {
		t.Fail()
	}
	_, err = f.Modify(nil, []uint64{0, 1})
	if err != nil {
		t.Log(err)
		t.Fatal("could not delete leaves 1 and 0")
	}
}

func TestExtractRow(t *testing.T) {
	targets := []uint64{4, 20, 24}
	forestRow := uint8(4)

	//fmt.Println(extractRow(targets, forestRow))

	fmt.Println("targets", targets)
	fmt.Println(extractRow(targets, forestRow, 2))
	fmt.Println("targets", targets)
	//fmt.Println(extractRow(targets, forestRow, 1))
	//fmt.Println(extractRow(targets, forestRow, 2))
}

func TestForestAddDel(t *testing.T) {
	numAdds := uint32(10)

	f := NewForest(RamForest, nil, "", 0)

	sc := newSimChain(0x07)

	allProof := 0

	for b := 0; b < 1000; b++ {

		adds, _, delHashes := sc.NextBlock(numAdds)

		bp, err := f.ProveBatch(delHashes)
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.Modify(adds, bp.Targets)
		if err != nil {
			t.Fatal(err)
		}

		allProof += len(bp.Proof)

		//fmt.Printf("nl %d %s", f.numLeaves, f.ToString())
	}

	fmt.Println("allproof", allProof)
}

func TestForestSwaplessAddDel(t *testing.T) {
	numAdds := uint32(3)

	allLeaves := make(map[Hash]interface{})

	f := NewForest(RamForest, nil, "", 0)

	sc := newSimChain(0x07)

	allProof := 0

	for b := 0; b < 1000; b++ {
		fmt.Println("start block ", b)
		for h := uint8(0); h < f.rows; h++ {
			if (f.numLeaves>>h)&1 == 1 {
				pos := rootPosition(f.numLeaves, h, f.rows)
				fmt.Println()
				fmt.Printf("pos %d, numLeaves %d, f.rows %d, root at row %d\n",
					pos, f.numLeaves, f.rows, h)
				fmt.Println(f.SubTreeToString(pos))
				fmt.Println()
			}
		}
		adds, _, delHashes := sc.NextBlock(numAdds)

		bp, err := f.ProveBatch(delHashes)
		if err != nil {
			t.Fatalf("TestSwapLessAddDel fail at block %d. Error: %v", b, err)
		}

		for _, del := range bp.Targets {
			//_, found := allDelPos[del]
			//if found {
			//	t.Fatalf("TestSwapLessAddDel fail. Re-deleting position %d", del)
			//}
			//allDelPos[del] = nil

			readHash := f.data.read(del)
			if readHash == empty {
				for h := uint8(0); h < f.rows; h++ {
					if (f.numLeaves>>h)&1 == 1 {
						pos := rootPosition(f.numLeaves, h, f.rows)
						fmt.Println()
						fmt.Printf("pos %d, numLeaves %d, f.rows %d, root at row %d\n",
							pos, f.numLeaves, f.rows, h)
						fmt.Println(f.SubTreeToString(pos))
						fmt.Println()
					}
				}
				t.Fatalf("Trying to delete empty at pos %d, block %d", del, b)
			}
		}

		_, err = f.ModifySwapless(adds, bp.Targets)
		if err != nil {
			fmt.Println(f.SubTreeToString(138))
			t.Fatalf("TestSwapLessAddDel fail at block %d. Error: %v", b, err)
		}

		err = f.PosMapSanitySwapless()
		if err != nil {
			for h := uint8(0); h < f.rows; h++ {
				if (f.numLeaves>>h)&1 == 1 {
					pos := rootPosition(f.numLeaves, h, f.rows)
					fmt.Println()
					fmt.Println("bp targets", bp.Targets)
					fmt.Printf("pos %d, numLeaves %d, f.rows %d, root at row %d\n",
						pos, f.numLeaves, f.rows, h)
					fmt.Println(f.SubTreeToString(pos))
					fmt.Println()
				}
			}
			t.Fatalf("TestSwapLessAddDel fail at block %d. Error: %v", b, err)
		}

		roots := f.GetRoots()
		for i, root := range roots {
			if root == empty {
				fmt.Println(f.ToString())
				for h := uint8(0); h < f.rows; h++ {
					if (f.numLeaves>>h)&1 == 1 {
						pos := rootPosition(f.numLeaves, h, f.rows)
						fmt.Println()
						fmt.Printf("pos %d, numLeaves %d, f.rows %d, root at row %d\n",
							pos, f.numLeaves, f.rows, h)
						fmt.Println(f.SubTreeToString(pos))
						fmt.Println()
					}
				}
				fmt.Printf("root %d is empty\n", i)
				//t.Fatalf("TestSwapLessAddDel fail: root %d is empty", i)
			}
		}

		//fmt.Println("ROOTS")
		//for i, root := range roots {
		//	fmt.Printf("root %d: %s\n", i, hex.EncodeToString(root[:]))
		//}
		//fmt.Println("ROOTS END")

		for h := uint8(0); h < f.rows; h++ {
			if (f.numLeaves>>h)&1 == 1 {
				pos := rootPosition(f.numLeaves, h, f.rows)
				fmt.Println()
				fmt.Printf("pos %d, numLeaves %d, f.rows %d, root at row %d\n",
					pos, f.numLeaves, f.rows, h)
				fmt.Println(f.SubTreeToString(pos))
				fmt.Println()
			}
		}

		//fmt.Printf("nl %d %s", f.numLeaves, f.ToString())

		for _, add := range adds {
			allLeaves[add.Hash] = nil
		}

		for _, del := range delHashes {
			delete(allLeaves, del)
		}

		for hash := range allLeaves {
			pos, found := f.positionMap[hash.Mini()]
			if !found {
				err := fmt.Errorf("Hash %s not present in the position map",
					hex.EncodeToString(hash[:]))
				t.Fatalf("TestSwapLessAddDel fail at block %d. Error: %v", b, err)
			}

			gotHash := f.data.read(pos)
			if gotHash != hash {
				err := fmt.Errorf("At position %d, expected %s, got %s",
					pos, hex.EncodeToString(hash[:]), hex.EncodeToString(gotHash[:]))
				t.Fatalf("TestSwapLessAddDel fail at block %d. Error: %v", b, err)
			}
		}

		allProof += len(bp.Proof)

		for hash, pos := range f.positionMap {
			readHash := f.data.read(pos)
			if hash != readHash.Mini() {
				err := fmt.Errorf("At position %d, position map had %s, read %s",
					pos, hex.EncodeToString(hash[:]), hex.EncodeToString(readHash[:]))
				t.Fatalf("TestSwapLessAddDel fail at block %d. Error: %v", b, err)
			}

			err := f.checkPosBelowAreEmpty(pos)
			if err != nil {
				t.Fatalf("TestSwapLessAddDel fail at block %d. Error: %v", b, err)
			}
		}
	}

	fmt.Println("swapless allproof", allProof)
}

func (f *Forest) checkPosBelowAreEmpty(origPosition uint64) error {
	fromRow := int(detectRow(origPosition, f.rows))

	positions := []uint64{origPosition}

	for currentRow := fromRow; currentRow >= 0; currentRow-- {
		nextPositions := []uint64{}

		for _, position := range positions {
			// Check children and add to the list of dels.
			leftChild := child(position, f.rows)
			rightChild := rightSib(leftChild)

			nextPositions = append(nextPositions, leftChild, rightChild)

			leftChildHash := f.data.read(leftChild)
			if currentRow != 0 && leftChildHash != empty {
				err := fmt.Errorf("Descendent %d of %d position not empty, read %s",
					origPosition, leftChild, hex.EncodeToString(leftChildHash[:]))
				return err
			}

			rightChildHash := f.data.read(rightChild)
			if currentRow != 0 && rightChildHash != empty {
				err := fmt.Errorf("Descendent %d of %d position not empty, read %s",
					origPosition, rightChild, hex.EncodeToString(rightChildHash[:]))
				return err
			}
		}

		positions = nextPositions
	}

	return nil
}

func TestGetRootPos(t *testing.T) {
	fmt.Println(getRootPosition(0, 15, 4))
	fmt.Println(getRootPosition(0, 31, 5))
}

func TestSubTreeToString(t *testing.T) {
	f := NewForest(RamForest, nil, "", 0)

	leaves := []Leaf{
		{Hash: Hash{1}},
		{Hash: Hash{2}},
		{Hash: Hash{3}},
		{Hash: Hash{4}},
		{Hash: Hash{5}},
		{Hash: Hash{6}},
		{Hash: Hash{7}},
		{Hash: Hash{8}},
		{Hash: Hash{9}},
		{Hash: Hash{9}},
		{Hash: Hash{10}},
		{Hash: Hash{11}},
		{Hash: Hash{12}},
		{Hash: Hash{13}},
		{Hash: Hash{14}},
		{Hash: Hash{15}},
		{Hash: Hash{16}},
		{Hash: Hash{17}},
		{Hash: Hash{18}},
		{Hash: Hash{19}},
		{Hash: Hash{20}},
		{Hash: Hash{21}},
	}

	_, err := f.ModifySwapless(leaves, nil)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println(f.ToString())

	for h := uint8(0); h < f.rows; h++ {
		if (f.numLeaves>>h)&1 == 1 {
			pos := rootPosition(f.numLeaves, h, f.rows)
			fmt.Println()
			fmt.Println("pos ", pos)
			fmt.Println(f.SubTreeToString(pos))
			fmt.Println()
		}
	}

	_, err = f.ModifySwapless(nil, []uint64{2, 3, 4, 5, 6, 7})
	if err != nil {
		t.Fatal(err)
	}

	for h := uint8(0); h < f.rows; h++ {
		if (f.numLeaves>>h)&1 == 1 {
			pos := rootPosition(f.numLeaves, h, f.rows)
			fmt.Println()
			fmt.Println("pos ", pos)
			fmt.Println(f.SubTreeToString(pos))
			fmt.Println()
		}
	}
}

func TestCowForestAddDelComp(t *testing.T) {
	// Function for writing logs.
	writeLog := func(cowF, memF *Forest) {
		cowstring := fmt.Sprintf("cowForest: nl %d %s\n",
			cowF.numLeaves, cowF.ToString())
		fmt.Println(cowstring)

		memstring := fmt.Sprintf("memForest: nl %d %s\n",
			memF.numLeaves, memF.ToString())
		fmt.Println(memstring)
	}

	tmpDir := os.TempDir()
	defer os.RemoveAll(tmpDir)

	cowF := NewForest(CowForest, nil, tmpDir, 2500)
	memF := NewForest(RamForest, nil, "", 0)
	numAdds := uint32(1000)

	sc := newSimChain(0x07)
	sc.lookahead = 400

	for b := 0; b <= 1000; b++ {
		adds, _, delHashes := sc.NextBlock(numAdds)

		cowBP, err := cowF.ProveBatch(delHashes)
		if err != nil {
			t.Fatal(err)
		}

		memBP, err := memF.ProveBatch(delHashes)
		if err != nil {
			t.Fatal(err)
		}
		_, err = cowF.Modify(adds, cowBP.Targets)
		if err != nil {
			t.Fatal(err)
		}
		_, err = memF.Modify(adds, memBP.Targets)
		if err != nil {
			t.Fatal(err)
		}
		if b%100 == 0 {
			err := cowF.AssertEqual(memF)
			if err != nil {
				writeLog(cowF, memF)
				t.Fatal(err)
			}
		}
	}

	err := cowF.AssertEqual(memF)
	if err != nil {
		writeLog(cowF, memF)
		t.Fatal(err)
	}
}

func TestCowForestAddDel(t *testing.T) {
	numAdds := uint32(10)

	tmpDir := os.TempDir()
	cowF := NewForest(CowForest, nil, tmpDir, 500)

	sc := newSimChain(0x07)
	sc.lookahead = 400

	for b := 0; b < 1000; b++ {

		adds, _, delHashes := sc.NextBlock(numAdds)

		cowBP, err := cowF.ProveBatch(delHashes)
		if err != nil {
			t.Fatal(err)
		}
		_, err = cowF.Modify(adds, cowBP.Targets)
		if err != nil {
			t.Fatal(err)
		}

		fmt.Printf("nl %d %s\n", cowF.numLeaves, cowF.ToString())
	}
}

func TestForestFixed(t *testing.T) {
	f := NewForest(RamForest, nil, "", 0)
	numadds := 5
	numdels := 3
	adds := make([]Leaf, numadds)
	dels := make([]uint64, numdels)

	for j, _ := range adds {
		adds[j].Hash[0] = uint8(j >> 8)
		adds[j].Hash[1] = uint8(j)
		adds[j].Hash[3] = 0xaa
	}
	for k, _ := range dels {
		dels[k] = uint64(k)
	}

	_, err := f.Modify(adds, nil)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Print(f.ToString())
	fmt.Print(f.PrintPositionMap())
	_, err = f.Modify(nil, dels)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Print(f.ToString())
	fmt.Print(f.PrintPositionMap())
}

// Add 2. delete 1.  Repeat.
func Test2Fwd1Back(t *testing.T) {
	f := NewForest(RamForest, nil, "", 0)
	var absidx uint32
	adds := make([]Leaf, 2)

	for i := 0; i < 100; i++ {

		for j, _ := range adds {
			adds[j].Hash[0] = uint8(absidx>>8) | 0xa0
			adds[j].Hash[1] = uint8(absidx)
			adds[j].Hash[3] = 0xaa
			absidx++
			//		if i%30 == 0 {
			//			utree.Track(adds[i])
			//			trax = append(trax, adds[i])
			//		}
		}

		//		t.Logf("-------- block %d\n", i)
		fmt.Printf("\t\t\t########### block %d ##########\n\n", i)

		// add 2
		_, err := f.Modify(adds, nil)
		if err != nil {
			t.Fatal(err)
		}

		s := f.ToString()
		fmt.Printf(s)

		// get proof for the first
		_, err = f.Prove(adds[0].Hash)
		if err != nil {
			t.Fatal(err)
		}

		// delete the first
		//		err = f.Modify(nil, []util.Hash{p.Payload})
		//		if err != nil {
		//			t.Fatal(err)
		//		}

		//		s = f.ToString()
		//		fmt.Printf(s)

		// get proof for the 2nd
		keep, err := f.Prove(adds[1].Hash)
		if err != nil {
			t.Fatal(err)
		}
		// check proof

		worked := f.Verify(keep)
		if !worked {
			t.Fatalf("proof at position %d, length %d failed to verify\n",
				keep.Position, len(keep.Siblings))
		}
	}
}

// Add and delete variable numbers, repeat.
// deletions are all on the left side and contiguous.
func TestAddxDelyLeftFullBatchProof(t *testing.T) {
	for x := 0; x < 10; x++ {
		for y := 0; y < x; y++ {
			err := addDelFullBatchProof(x, y)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

}

// Add x, delete y, construct & reconstruct blockproof
func addDelFullBatchProof(nAdds, nDels int) error {
	if nDels > nAdds-1 {
		return fmt.Errorf("too many deletes")
	}

	f := NewForest(RamForest, nil, "", 0)
	adds := make([]Leaf, nAdds)

	for j, _ := range adds {
		adds[j].Hash[0] = uint8(j>>8) | 0xa0
		adds[j].Hash[1] = uint8(j)
		adds[j].Hash[3] = 0xaa
	}

	// add x
	_, err := f.Modify(adds, nil)
	if err != nil {
		return err
	}
	addHashes := make([]Hash, len(adds))
	for i, h := range adds {
		addHashes[i] = h.Hash
	}

	leavesToProve := addHashes[:nDels]

	// get block proof
	bp, err := f.ProveBatch(leavesToProve)
	if err != nil {
		return err
	}
	// check block proof.  Note this doesn't delete anything, just proves inclusion
	_, _, err = verifyBatchProof(leavesToProve, bp, f.GetRoots(), f.numLeaves, nil)
	if err != nil {
		return fmt.Errorf("VerifyBatchProof failed. Error: %s", err.Error())
	}
	fmt.Printf("VerifyBatchProof worked\n")
	return nil
}

func TestDeleteNonExisting(t *testing.T) {
	f := NewForest(RamForest, nil, "", 0)
	deletions := []uint64{0}
	_, err := f.Modify(nil, deletions)
	if err == nil {
		t.Fatal(fmt.Errorf(
			"shouldn't be able to delete non-existing leaf 0 from empty forest"))
	}
}

func TestSmallRandomForests(t *testing.T) {
	rand := rand.New(rand.NewSource(0))

	for i := 0; i < 1000; i++ {
		// The forest instance to test in this iteration of the loop
		f := NewForest(RamForest, nil, "", 0)

		// We use 'quick' to generate testing data:
		// we interpret the keys as leaf hashes and the values
		// as indicating whether we should delete the leaf on
		// our second call to Modify
		value, ok := quick.Value(reflect.TypeOf((map[uint8]bool)(nil)), rand)
		if !ok {
			t.Fatal("could not create uint8->bool map")
		}
		adds := value.Interface().(map[uint8]bool)

		// This is the leaf that we will test proofs for
		// if we happen to generate testing data that
		// doesn't leave an empty tree.
		var chosenUndeletedLeaf Leaf
		atLeastOneLeafRemains := false

		// forest.Modify takes a slice, so we'll copy
		// adds into this slice:
		addsSlice := make([]Leaf, len(adds))

		// This stores the leaves that are to be deleted.
		// We need to store the LeafTXO's or we won't be able
		// to find the positions after inserting all items.
		leavesToDeleteSet := make(map[Leaf]struct{})

		i := 0
		for firstLeafHashByte, deleteLater := range adds {
			// We put a one in the hash too, so that we won't
			// generate an all-zero hash, which is not allowed.
			leafTxo := Leaf{Hash: Hash{firstLeafHashByte, 1}}
			addsSlice[i] = leafTxo
			if deleteLater {
				leavesToDeleteSet[leafTxo] = struct{}{}
			} else {
				atLeastOneLeafRemains = true
				chosenUndeletedLeaf = leafTxo
			}
			i++
		}

		_, err := f.Modify(addsSlice, nil)
		if err != nil {
			t.Fatalf("could not add leafs to empty forest: %v", err)
		}

		// We convert leavesToDeleteSet to an array, so we
		// can sort it.
		// Modify requires a sorted list of leaves to delete.
		// We use int because we can't sort uint64's.
		deletions := make([]int, len(leavesToDeleteSet))
		i = 0
		for leafTxo, _ := range leavesToDeleteSet {
			deletions[i] = int(f.positionMap[leafTxo.Mini()])
			i++
		}
		sort.Ints(deletions)

		// We convert to uint64 so that we can call Modify
		deletions_uint64 := make([]uint64, len(deletions))
		i = 0
		for _, el := range deletions {
			deletions_uint64[i] = uint64(el)
			i++
		}
		t.Logf("\nadding (the bool values are whether deletion happens):\n%v\ndeleting:\n%v\n", adds, deletions_uint64)

		_, err = f.Modify(nil, deletions_uint64)
		if err != nil {
			t.Fatalf("could not delete leaves from filled forest: %v", err)
		}

		// If the tree we filled isn't empty, and contains a node we didn't delete,
		// we should be able to make a proof for that leaf
		if atLeastOneLeafRemains {
			blockProof, err := f.ProveBatch(
				[]Hash{
					chosenUndeletedLeaf.Hash,
				})
			if err != nil {
				t.Fatalf("proveblock failed proving existing leaf: %v", err)
			}

			err = f.VerifyBatchProof([]Hash{chosenUndeletedLeaf.Hash}, blockProof)
			if err != nil {
				retErr := fmt.Errorf("verifyblockproof failed verifying proof for existing leaf."+
					" Error: %s", err.Error())
				t.Fatal(retErr)
			}
		}
	}
}
