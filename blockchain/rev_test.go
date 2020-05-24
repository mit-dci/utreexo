package blockchain

import (
	"fmt"
	"os"
	"testing"
)

/*
 * TODO: These tests require rev*.dat files in the util directory.
 * Copy them over to test.
 */

func TestGetRevBlocks(t *testing.T) {
	// Makes neccessary directories
	MakePaths()

	// Builds an index
	// takes less than 1/10th of a  second
	err := BuildRevOffsetFile()
	if err != nil {
		t.Fatal(err)
	}

	// Gets blocks 1 ~ 300,001
	for i := int32(0); i < 300000; i++ {
		rb, err := GetRevBlock(i, RevOffsetFilePath)
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
		fmt.Println("height", i+1)
		for _, tx := range rb.Block.Tx {
			for i, txin := range tx.TxIn {
				fmt.Println("txcount:", i)
				fmt.Println(txin)
			}
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

	// Any arbitrary block will do here for testing
	// 382 actually fetches block 383
	rb, err := GetRevBlock(382, RevOffsetFilePath)
	for _, tx := range rb.Block.Tx {
		for _, txin := range tx.TxIn {
			fmt.Println(txin)
		}
	}
	if err != nil {
		t.Log("Failed at height:", 382+1)
		os.RemoveAll(OffsetDirPath)
		os.RemoveAll(ProofDirPath)
		os.RemoveAll(ForestDirPath)
		os.RemoveAll(PollardDirPath)
		os.RemoveAll(RevOffsetDirPath)
		t.Fatal(err)
	}
	os.RemoveAll(OffsetDirPath)
	os.RemoveAll(ProofDirPath)
	os.RemoveAll(ForestDirPath)
	os.RemoveAll(PollardDirPath)
	os.RemoveAll(RevOffsetDirPath)
}
