package accumulator

import (
	"bytes"
	"fmt"
	"testing"
)

func TestGPosToLocPos(t *testing.T) {
	pos := uint64(254)
	forestRows := uint8(7)
	treeBlockRow, offset, err := getTreeBlockPos(pos, forestRows)
	if err != nil {
		t.Fatal(err)
	}

	locRow, locPos := gPosToLocPos(pos, offset, treeBlockRow, forestRows)
	fmt.Printf("\nfor gPos:%d, treeBlockRow:%d, offset:%d, forestRows:%d -->\n"+
		"locRow:%d, locPos:%d\n", pos, treeBlockRow, offset, forestRows,
		locRow, locPos)
	fmt.Printf("\ntreeBlockRow: %d, offset: %d, err: %s\n",
		treeBlockRow, offset, err)

}

func TestGetTreeBlockPos(t *testing.T) {
	//pos := uint64(4)
	//forestRows := uint8(3)
	//maxCachedTables := 1

	//pos := uint64(1040384)
	pos := uint64(67108860)
	forestRows := uint8(25)
	treeBlockRow, offset, err := getTreeBlockPos(pos, forestRows)
	fmt.Printf("For pos: %d, forestRows: %d\n", pos, forestRows)
	fmt.Printf("treeBlockRow: %d, offset: %d, err: %s\n",
		treeBlockRow, offset, err)

}

func TestGetRowOffset(t *testing.T) {
	for forestRows := uint8(0); forestRows <= 63; forestRows++ {
		for row := uint8(0); row <= forestRows; row++ {
			offset := getRowOffset(row, forestRows)

			offsetcomp := testGetRowOffset(row, forestRows)

			if offsetcomp != offset {
				t.Fatal()
			}

		}
	}
}

// a easier version of getRowOffset to grasp
func testGetRowOffset(row, forestRows uint8) uint64 {
	var offset uint64
	leaves := uint64(1 << forestRows)

	for i := uint8(0); i < row; i++ {
		offset += leaves
		leaves /= 2
	}

	return offset
}

func TestTreeTableSerialize(t *testing.T) {
	newtt := treeTable{}

	for n := 0; n < treeBlockPerTable; n++ {
		newtb := treeBlock{}

		for i := uint(0); i < 127; i++ {
			h := new(Hash)
			h[0] = 1 << i
			newtb.leaves[i] = *h
		}

		newtt.memTreeBlocks[n] = &newtb
	}

	treeBlockCount, buf := newtt.serialize()
	bufR := bytes.NewReader(buf)
	deserTable, err := deserializeTreeTable(bufR, treeBlockCount)
	if err != nil {
		t.Fatal(err)
	}

	treeBlockCountAfter, bufAfter := deserTable.serialize()

	if bytes.Compare(bufAfter, buf) != 0 {
		t.Fatal("treeTables not equal after serialization and deserializiation")
	}

	if treeBlockCountAfter != treeBlockCount {
		t.Fatal("treeBlockCount not equal after serial and deserial")
	}

}
