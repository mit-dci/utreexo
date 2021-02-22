package accumulator

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"testing"
	"testing/quick"
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
	// Create a table
	newtt := treeTable{}
	for n := 0; n < treeBlockPerTable; n++ {
		newtb := treeBlock{}

		for i := uint(0); i < nodesPerTreeBlock; i++ {
			h := new(Hash)
			h[0] = 1 << i
			newtb.leaves[i] = *h
		}

		newtt.memTreeBlocks[n] = &newtb
	}

	// Serialize the table
	var buf []byte
	newtt.serialize(&buf)

	// Grab the 2 bytes of treeBlockCount that was serialized
	lenBytes := make([]byte, 2)
	copy(lenBytes, buf[0:2])
	treeBlockCount := binary.LittleEndian.Uint16(lenBytes)

	// create copy of the buf and make a reader to deserialize
	readerBuf := make([]byte, len(buf))
	copy(readerBuf, buf)
	bufR := bytes.NewReader(readerBuf)

	// Deserialize
	deserializedTable, err := deserializeTreeTable(bufR)
	if err != nil {
		t.Fatal(err)
	}

	// Re-serialize the deserializedTable
	var bufAfter []byte
	deserializedTable.serialize(&bufAfter)
	treeBlockCountAfter := binary.LittleEndian.Uint16(bufAfter[0:2])

	// Compare
	if bytes.Compare(bufAfter, buf) != 0 {
		fmt.Println(len(bufAfter))
		fmt.Println(len(buf))
		t.Fatal("treeTables not equal after serialization and deserializiation")
	}

	if treeBlockCountAfter != treeBlockCount {
		t.Fatal("treeBlockCount not equal after serial and deserial")
	}

}

// creates a pseudo-random hash from a given int64 source
func createRandomHash(i int64) [32]byte {
	rand := rand.New(rand.NewSource(i))
	value, ok := quick.Value(reflect.TypeOf(([]byte)(nil)), rand)
	if !ok {
		panic("could not create rand value to hash")
	}
	toBeHashed := value.Interface().([]byte)
	return sha256.Sum256(toBeHashed)
}

func TestCowForestWrite(t *testing.T) {
	// keep only 1 treetable in memory to force flush and
	// test the flushing/restoring as well
	f := NewForest(nil, false, os.TempDir(), 1)

	numAdds := uint32(10)   // adds per block
	sc := NewSimChain(0x07) // A chain simulator

	// go through block 0 to 1000 and test the add/dels
	for blockNum := 0; blockNum < 250; blockNum++ {
		// Creates add/dels from the chain simulator
		adds, _, delHashes := sc.NextBlock(numAdds)

		// Create batchproof
		bp, err := f.ProveBatch(delHashes)
		if err != nil {
			t.Fatal(err)
		}

		// Modify cowForest
		_, err = f.Modify(adds, bp.Targets)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Go through the bottom row and write/read
	for i := uint64(0); i < f.numLeaves; i++ {
		hash := createRandomHash(int64(i))

		f.data.write(i, hash)
		if hash != f.data.read(i) {
			str := fmt.Errorf("Written hash: %v at position: %v but"+
				"read hash %v\n", hash, i, f.data.read(i))
			t.Fatal(str)
		}
	}
}
