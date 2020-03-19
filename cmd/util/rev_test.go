package util

import (
	"fmt"
	"os"
	"testing"
)

func TestGetRevBlocks(t *testing.T) {
	// Makes neccessary directories
	MakePaths()

	// Builds an index
	// takes less than 1/10th of a  second
	err := BuildRevOffsetFile()
	if err != nil {
		t.Fatal(err)
	}

	// Gets blocks 1 ~ 700,000
	for i := int32(0); i < 700000; i++ {
		_, err := GetRevBlock(i, RevOffsetFilePath)
		if err != nil {
			t.Log("Failed at height:", i+1)
			// If it does fail, delete the created directories
			os.RemoveAll(OffsetDirPath)
			os.RemoveAll(ProofDirPath)
			os.RemoveAll(ForestDirPath)
			os.RemoveAll(PollardDirPath)
			os.RemoveAll(RevOffsetDirPath)
			t.Fatal(err)
		}
	}
	// Delete all the directories
	os.RemoveAll(OffsetDirPath)
	os.RemoveAll(ProofDirPath)
	os.RemoveAll(ForestDirPath)
	os.RemoveAll(PollardDirPath)
	os.RemoveAll(RevOffsetDirPath)
}

func TestGetOneRevBlock(t *testing.T) {
	MakePaths()

	err := BuildRevOffsetFile()
	if err != nil {
		t.Fatal(err)
	}

	rb, err := GetRevBlock(382, RevOffsetFilePath) // fetches block 383
	if err != nil {
		t.Log("Failed at height:", 382+1)
		os.RemoveAll(OffsetDirPath)
		os.RemoveAll(ProofDirPath)
		os.RemoveAll(ForestDirPath)
		os.RemoveAll(PollardDirPath)
		os.RemoveAll(RevOffsetDirPath)
		t.Fatal(err)
	}
	fmt.Println("Block:", rb.Block)
	fmt.Println("Txs:", rb.Block.Tx)
	os.RemoveAll(OffsetDirPath)
	os.RemoveAll(ProofDirPath)
	os.RemoveAll(ForestDirPath)
	os.RemoveAll(PollardDirPath)
	os.RemoveAll(RevOffsetDirPath)
}
