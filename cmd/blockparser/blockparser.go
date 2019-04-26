package main

import (
	"fmt"
	"os"

	"github.com/mit-dci/lit/btcutil/chaincfg/chainhash"
	"github.com/mit-dci/lit/wire"
)

/*
Read bitcoin core's levelDB index folder, and the blk000.dat files with
the blocks.
*/
type Hash [32]byte

var mainnetGenHash = Hash{
	0x6f, 0xe2, 0x8c, 0x0a, 0xb6, 0xf1, 0xb3, 0x72,
	0xc1, 0xa6, 0xa2, 0x46, 0xae, 0x63, 0xf7, 0x4f,
	0x93, 0x1e, 0x83, 0x65, 0xe1, 0x5a, 0x08, 0x9c,
	0x68, 0xd6, 0x19, 0x00, 0x00, 0x00, 0x00, 0x00,
}

var testNet3GenHash = Hash{
	0x43, 0x49, 0x7f, 0xd7, 0xf8, 0x26, 0x95, 0x71,
	0x08, 0xf4, 0xa3, 0x0f, 0xd9, 0xce, 0xc3, 0xae,
	0xba, 0x79, 0x97, 0x20, 0x84, 0xe9, 0x0e, 0xad,
	0x01, 0xea, 0x33, 0x09, 0x00, 0x00, 0x00, 0x00,
}

func main() {
	fmt.Printf("hi\n")

	err := parser()
	if err != nil {
		panic(err)
	}
}

func parser() error {

	fileNum := 0
	fileName := fmt.Sprintf("blk%05d.dat", fileNum)

	fmt.Printf("filename %s\n", fileName)

	blocks, err := readRawBlocksFromFile(fileName)
	if err != nil {
		return err
	}

	err = sortBlocks(blocks)
	if err != nil {
		return err
	}

	return nil
}

func sortBlocks(blocks []wire.MsgBlock) error {

	tip := blocks[0].BlockHash()
	tipnum := 0

	nextMap := make(map[chainhash.Hash]wire.MsgBlock)

	for x, b := range blocks {
		fmt.Printf("read %d, tip %d map %d\n", x, tipnum, len(nextMap))
		if len(nextMap) > 1000 {
			fmt.Printf("tip at %d %s\n", tipnum, tip.String())
			break
		}
		// skip first block
		if x == 0 {
			tip = b.BlockHash()
			tipnum++
			continue
		}

		if b.Header.PrevBlock != tip { // not in line, add to map
			nextMap[b.Header.PrevBlock] = b
			continue
		}

		// inline, progress tip and deplete map
		tip = b.BlockHash()
		tipnum++

		// check for next blocks in map
		stashedBlock, ok := nextMap[tip]
		for ok {
			tip = stashedBlock.BlockHash()
			tipnum++

			fmt.Printf("%s in map and points to %s\n",
				stashedBlock.BlockHash().String(), tip.String())
			delete(nextMap, stashedBlock.Header.PrevBlock)

			stashedBlock, ok = nextMap[tip]
		}
	}

	return nil
}

func readRawBlocksFromFile(fileName string) ([]wire.MsgBlock, error) {
	var blocks []wire.MsgBlock

	f, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}

	fstat, err := f.Stat()
	if err != nil {
		return nil, err
	}

	loc := int64(0) // presumably we start at offset 0

	for loc != fstat.Size() {
		_, err = f.Seek(8, 1)
		if err != nil {
			return nil, err
		}

		b := new(wire.MsgBlock)
		err = b.Deserialize(f)
		if err != nil {
			return nil, err
		}

		blocks = append(blocks, *b)
		loc, err = f.Seek(0, 1)
		if err != nil {
			return nil, err
		}
	}

	return blocks, nil
}
