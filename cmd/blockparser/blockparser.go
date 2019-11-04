package blockparser

import (
	"fmt"
	"io/ioutil"
	"os"
	"sync"

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

	// append if testnet.txos exists. Create one if it doesn't exist
	outfile, err := os.OpenFile("testnet.txos", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}

	progressfile, err := os.OpenFile("progress", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	pinfo, err := progressfile.Stat()
	if err != nil {
		return err
	}

	var lastblock blockInfo
	var tipnum uint32
	var offset int64
	var tipHash chainhash.Hash

	if pinfo.Size() == 44 {
		lastblock, tipHash, err = getTipInfo(progressfile)
		if err != nil {
			return err
		}
		tipnum = lastblock.height
		offset = int64(lastblock.offset)
	} else {
		tipHash = testNet3GenHash
		// everything else is 0
	}

	go stopParse(sig)

	nextMap := make(map[chainhash.Hash]blockInfo)
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

	for fileNum := lastblock.fnum; ; fileNum++ {
		blocks, err := readRawBlocksFromFile(fileNum, offset)
		if err != nil {
			return err
		}
		if blocks == nil {
			break
		}
		offset = 0 // only applies to first file read

		tipHash, tipnum, err = sortBlocks(
			blocks, nextMap, tipHash, tipnum, outfile, progressfile, batchan, &batchwg)
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

type blockInfo struct {
	wire.MsgBlock
	height uint32 // height within the blockchain
	fnum   uint32 // file number from where this block came from
	offset uint32 // byte offset -- the byte AFTER this block ends
	// NOTE that because we use uint32s here this doesn't support files of
	// more than 4GB (they all seem to be 128MB) or more than 4 billion
	// block files.  (seems pretty unlikely as that'd be half an exabyte)
}

func readRawBlocksFromFile(fileNum uint32, offset int64) ([]blockInfo, error) {
	fileName := fmt.Sprintf("blk%05d.dat", fileNum)
	fmt.Printf("reading %s\n", fileName)

	_, err := os.Stat(fileName)
	if os.IsNotExist(err) {
		fmt.Printf("%s doesn't exist; done reading\n", fileName)
		return nil, nil
	}

	var blocks []blockInfo

	f, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}

	fstat, err := f.Stat()
	if err != nil {
		return nil, err
	}

	f.Seek(offset, 0)
	blockInFile := 0
	var prevHdr wire.BlockHeader
	for offset != fstat.Size() {
		var magicbytes [4]byte
		f.Read(magicbytes[:])
		if magicbytes != [4]byte{0x0b, 0x11, 0x09, 0x07} && //testnet
			magicbytes != [4]byte{0xf9, 0xbe, 0xb4, 0xd9} { // mainnet
			fmt.Printf("got non magic bytes %x, finishing\n", magicbytes)
			fmt.Printf("offset %d file %d\n", offset, fileNum)
			break
		}

		//reads 4 bytes from the current offset
		offset, err = f.Seek(4, 1)
		if err != nil {
			return nil, err
		}

		b := new(wire.MsgBlock)
		err = b.Deserialize(f)
		if err != nil {
			fmt.Printf("prev idx %d hash %s file %s offset %d\n",
				blockInFile, prevHdr.BlockHash().String(), fileName, offset)
			return nil, err
		}

		offset, err = f.Seek(0, 1) // gets the offset we're at now
		if err != nil {
			return nil, err
		}

		// height unknown yet, but filenum and offset are known
		blocks = append(blocks,
			blockInfo{MsgBlock: *b, fnum: fileNum, offset: uint32(offset)})

		prevHdr = b.Header
		blockInFile++
	}

	return blocks, nil
}

//sortBlocks sorts blocks from the .dat files inside blocks/ folder.
//Blocks by default are not in order when synced with bitcoin core.
//It also calls writeBlocks, which writes to the .txos file.
func sortBlocks(
	blocks []blockInfo,
	nextMap map[chainhash.Hash]blockInfo,
	tip chainhash.Hash, tipnum uint32,
	outfile *os.File,
	progressfile *os.File,
	batchan chan *leveldb.Batch,
	batchwg *sync.WaitGroup) (chainhash.Hash, uint32, error) {

	// var foffsetSlice []uint64
	// keep a slice of offsets of the blocks in ram (nextMap) and write
	// the earliest one to disk?
	for _, b := range blocks {
		if len(nextMap) > 10000 {
			fmt.Printf("dead-end tip at %d %s\n", tipnum, tip.String())
			break
		}

		if b.Header.PrevBlock != tip { // not in line, height unknown, add to map
			nextMap[b.Header.PrevBlock] = b
			continue
		}

		// inline, progress tip and deplete map
		tip = b.BlockHash()
		tipnum++
		b.height = tipnum
		err := writeBlock(b, outfile, progressfile, batchan, batchwg)
		if err != nil {
			return tip, tipnum, err
		}

		// check for next blocks in map
		stashedBlock, ok := nextMap[tip]
		for ok {
			tip = stashedBlock.BlockHash()
			tipnum++
			stashedBlock.height = tipnum
			err := writeBlock(stashedBlock, outfile, progressfile, batchan, batchwg)
			if err != nil {
				return tip, tipnum, err
			}

			delete(nextMap, stashedBlock.Header.PrevBlock)
			fmt.Printf("nextMap %d\n", len(nextMap))
			if len(nextMap) == 1 {
				for h, b := range nextMap {
					fmt.Printf("map has %x %d\n", h, b.height)
				}
			}

			stashedBlock, ok = nextMap[tip]
		}
	}
	fmt.Printf("tip %d map %d\n", tipnum, len(nextMap))
	return tip, tipnum, nil
}

// getTipNum resumes where the parsing left off.
// It starts from the beginning if progress file wasn't found.
// returns a blockinfo and a hash, as it doesn't have an acual block
func getTipInfo(progressfile *os.File) (blockInfo, chainhash.Hash, error) {
	var b blockInfo
	pbytes, err := ioutil.ReadAll(progressfile)
	if err != nil {
		return b, b.BlockHash(), err
	}
	b.height = BtU32(pbytes[0:4])
	b.fnum = BtU32(pbytes[4:8])
	b.offset = BtU32(pbytes[8:12])

	var bhash chainhash.Hash
	err = bhash.SetBytes(pbytes[12:44])
	return b, bhash, err
}

func dumpTipInfo(progressfile *os.File, b blockInfo) error {
	fmt.Printf("dumping h %d fn %d offset %d %s\n",
		b.height, b.fnum, b.offset, b.BlockHash().String())
	_, err := progressfile.WriteAt(U32tB(b.height), 0)
	if err != nil {
		return err
	}

	_, err = progressfile.WriteAt(U32tB(b.fnum), 4)
	if err != nil {
		return err
	}
	_, err = progressfile.WriteAt(U32tB(uint32(b.offset)), 8)
	if err != nil {
		return err
	}
	tipHash := b.BlockHash()
	_, err = progressfile.WriteAt(tipHash[:], 12)
	return err
}

//StopParse deletes the current block.
//Handles SIGTERM and SIGINT.
func stopParse(sig chan bool) {
	<-sig
	fmt.Println("Exiting...")
	os.Exit(1)
}
