package blockparser

import (
	"fmt"
	"io/ioutil"
	"os"
	"sync"
	"strconv"

	"github.com/mit-dci/lit/btcutil/chaincfg/chainhash"
	"github.com/mit-dci/lit/wire"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

/*
Read bitcoin core's levelDB index folder, and the blk000.dat files with
the blocks.

Currently, sortBlocks() reads from the .dat files and sorts the blocks.
Once the blocks are sorted, sortBlocks calls writeBlock() and it starts writing to the .txos file.

Writes a text txo file, and also creates a levelDB folder with the block
heights where every txo is consumed. These files feed in to txottl to
make a txo text file with the ttl times on each line
*/

var mainnetGenHash = chainhash.Hash{
	0x6f, 0xe2, 0x8c, 0x0a, 0xb6, 0xf1, 0xb3, 0x72,
	0xc1, 0xa6, 0xa2, 0x46, 0xae, 0x63, 0xf7, 0x4f,
	0x93, 0x1e, 0x83, 0x65, 0xe1, 0x5a, 0x08, 0x9c,
	0x68, 0xd6, 0x19, 0x00, 0x00, 0x00, 0x00, 0x00,
}

var testNet3GenHash = chainhash.Hash{
	0x43, 0x49, 0x7f, 0xd7, 0xf8, 0x26, 0x95, 0x71,
	0x08, 0xf4, 0xa3, 0x0f, 0xd9, 0xce, 0xc3, 0xae,
	0xba, 0x79, 0x97, 0x20, 0x84, 0xe9, 0x0e, 0xad,
	0x01, 0xea, 0x33, 0x09, 0x00, 0x00, 0x00, 0x00,
}

//Parser parses blocks from the .dat files bitcoin core provides
func Parser(sig chan bool) error {

	tipnum := getTipNum()
	tip := testNet3GenHash

	go stopParse(sig, tipnum)

	//append if testnet.txos exists. Create one if it doesn't exist
	outfile, err := os.OpenFile("testnet.txos", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}

	cb, err := os.OpenFile("currentblock.txos", os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}

	nextMap := make(map[chainhash.Hash]wire.MsgBlock)
	o := new(opt.Options)
	o.CompactionTableSizeMultiplier = 8
	lvdb, err := leveldb.OpenFile("./ttldb", o)
	if err != nil {
		panic(err)
	}
	defer lvdb.Close()
	var batchwg sync.WaitGroup
	// make the channel ... have a buffer? does it matter?
	batchan := make(chan *leveldb.Batch)

	//start db writer worker... actually start a bunch of em
	// try 16 workers...?
	for j := 0; j < 16; j++ {
		go dbWorker(batchan, lvdb, &batchwg)
	}

	for fileNum := 0; ; fileNum++ {
		fileName := fmt.Sprintf("blk%05d.dat", fileNum)
		fmt.Printf("reading %s\n", fileName)

		_, err := os.Stat(fileName)
		if os.IsNotExist(err) {
			fmt.Printf("%s doesn't exist; done reading\n", fileName)
			break
		}
		blocks, err := readRawBlocksFromFile(fileName)
		if err != nil {
			return err
		}

		tip, tipnum, err = sortBlocks(
			blocks, nextMap, tip, tipnum, outfile, cb, batchan, &batchwg)
		if err != nil {
			return err
		}
	}
	batchwg.Wait()

	return nil
}

//isUnspendable determines whether a tx is spenable or not.
//returns true if spendable, false if unspenable.
func isUnspendable(o *wire.TxOut) bool {
	switch {
	case len(o.PkScript) > 10000: //len 0 is OK, spendable
		return true
	case len(o.PkScript) > 0 && o.PkScript[0] == 0x6a: //OP_RETURN is 0x6a
		return true
	default:
		return false
	}
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
	blockInFile := 0
	var prevHdr wire.BlockHeader
	for loc != fstat.Size() {

		var magicbytes [4]byte
		f.Read(magicbytes[:])
		if magicbytes != [4]byte{0x0b, 0x11, 0x09, 0x07} && //testnet
			magicbytes != [4]byte{0xf9, 0xbe, 0xb4, 0xd9} { // mainnet
			fmt.Printf("got non magic bytes %x, finishing\n", magicbytes)
			break
		}

		//reads 4 bytes from the current offset
		loc, err = f.Seek(4, 1)
		if err != nil {
			return nil, err
		}

		b := new(wire.MsgBlock)
		err = b.Deserialize(f)
		if err != nil {
			fmt.Printf("prev idx %d hash %s file %s offset %d\n",
				blockInFile, prevHdr.BlockHash().String(), fileName, loc)
			return nil, err
		}

		blocks = append(blocks, *b)
		loc, err = f.Seek(0, 1)
		if err != nil {
			return nil, err
		}
		prevHdr = b.Header
		blockInFile++
	}

	return blocks, nil
}

//sortBlocks sorts blocks from the .dat files inside blocks/ folder.
//Blocks by default are not in order when synced with bitcoin core.
//It also calls writeBlocks, which writes to the .txos file.
func sortBlocks(
	blocks []wire.MsgBlock,
	nextMap map[chainhash.Hash]wire.MsgBlock,
	tip chainhash.Hash, tipnum int,
	outfile *os.File,
	cb *os.File,
	batchan chan *leveldb.Batch,
	batchwg *sync.WaitGroup) (chainhash.Hash, int, error) {

	for _, b := range blocks {
		if len(nextMap) > 10000 {
			fmt.Printf("dead-end tip at %d %s\n", tipnum, tip.String())
			break
		}

		if b.Header.PrevBlock != tip { // not in line, add to map
			nextMap[b.Header.PrevBlock] = b
			continue
		}

		// inline, progress tip and deplete map
		tip = b.BlockHash()
		tipnum++
		err := writeBlock(b, tipnum, outfile, cb, batchan, batchwg)
		if err != nil {
			return tip, tipnum, err
		}

		// check for next blocks in map
		stashedBlock, ok := nextMap[tip]
		for ok {
			tip = stashedBlock.BlockHash()
			tipnum++
			err := writeBlock(stashedBlock, tipnum, outfile, cb, batchan, batchwg)
			if err != nil {
				return tip, tipnum, err
			}

			delete(nextMap, stashedBlock.Header.PrevBlock)
			stashedBlock, ok = nextMap[tip]
		}
	}
	fmt.Printf("tip %d map %d\n", tipnum, len(nextMap))
	return tip, tipnum, nil
}

//getTipNum resumes where the parsing left off.
//It starts from the beginning if tipnum.txos file wasn't found.
func getTipNum() int {
	file, err := os.OpenFile("currentblock.txos", os.O_RDONLY, 0400)
	if err != nil {
		var tipnum int
		return tipnum
	}

	b, err := ioutil.ReadAll(file)
	tipnum, err := strconv.Atoi(string(b))
	fmt.Println(tipnum)
	return tipnum
}

//StopParse deletes the current block.
//Handles SIGTERM and SIGINT.
func stopParse(sig chan bool, tipnum int) {
	<-sig
	fmt.Println("Exiting...")
	os.Exit(1)
}
