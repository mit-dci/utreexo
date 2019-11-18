package blockparser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/mit-dci/lit/btcutil/chaincfg/chainhash"
	"github.com/mit-dci/lit/wire"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

/*
Read bitcoin core's levelDB index folder, and the blk000.dat files with
the blocks.

Writes a text txo file, and also creates a levelDB folder with the block
heights where every txo is consumed. These files feed in to txottl to
make a txo text file with the ttl times on each line
*/

//Header data read off the .dat file.
//FileNum is the .dat file number
//Offset is where it is in the .dat file.
//CurrentHeaderHash is the 80byte header double hashed
//Prevhash is the 32 byte previous header included in the 80byte header.
type RawHeaderData struct {
	FileNum [4]byte
	Offset [4]byte
	CurrentHeaderHash [32]byte
	Prevhash [32]byte
}

//chainhash.Hash is just [32]byte
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

	offsetfinished := make(chan bool, 1)

	//listen for SIGINT, SIGTERM, or SIGQUIT from the os
	go stopParse(sig, offsetfinished)

	var currentOffsetHeight int
	tipnum := 0
	tip := testNet3GenHash
	nextMap := make(map[[32]byte]RawHeaderData)

	//if there isn't an offset file, make one
	if hasAccess("offsetfile") == false {
		currentOffsetHeight, _ = buildOffsetFile(tip, tipnum, nextMap, offsetfinished)
	//if there is a offset file, we should pass true to offsetfinished
	//to let stopParse() know that it shouldn't delete offsetfile
	} else {
		offsetfinished <- true
	}
	//if there is a .txos file, get the tipnum from that
	if hasAccess("testnet.txos") == true {
		fmt.Println("Got tip number from .txos file")
		tipnum, _ = getTipNum()
	}

	//grab the last block height from currentoffsetheight
	//currentoffsetheight saves the last height from the offsetfile
	var currentOffsetHeightByte [4]byte
	currentOffsetHeightFile, err := os.Open("currentoffsetheight")
	if err != nil {
		panic(err)
	}
	currentOffsetHeightFile.Read(currentOffsetHeightByte[:])
	currentOffsetHeight = int(BtU32(currentOffsetHeightByte[:]))

	//append if testnet.txos exists. Create one if it doesn't exist
	outfile, err := os.OpenFile("testnet.txos", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}

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

	fmt.Println("Building the .txos file...")
	fmt.Println("Starting from block:", tipnum)
	//read off the offset file and start writing to the .txos file
	for ; tipnum != currentOffsetHeight; tipnum++ {
		offsetFile, err := os.Open("offsetfile")
		if err != nil {
			panic(err)
		}
		block, err := getRawBlockFromFile(tipnum, offsetFile)
		if err != nil {
			panic(err)
		}
		//write to the .txos file
		writeBlock(block, tipnum+1, outfile, batchan, &batchwg)
		//Just something to let the user know that the program is still running
		//The actual block the program is on is +1 of the printed number
		if tipnum % 50000 == 0 {
			fmt.Println("On block :", tipnum)
		}
	}
	batchwg.Wait()
	fmt.Println("Finished writing")
	return nil
}

//Gets the latest tipnum from the .txos file
func getTipNum() (int, error) {
	//check if there is access to the .txos file
	if hasAccess("testnet.txos") == false {
		fmt.Println("No testnet.txos file found")
		os.Exit(1)
	}

	f, err := os.Open("testnet.txos")
	if err != nil {
		panic(err)
	}

	//check if the .txos file is empty
	fstat, _ := f.Stat()
	if fstat.Size() == 0 {
		fmt.Println(".txos file is empty. Syncing from the genesis block...")
		return 0, nil
	}

	var all []byte
	buf := make([]byte, 1)
	var x int64
	var s string

	//Reads backwards and appends the character read to `all` until we hit the character "h".
	for s != "h" && x > -20 {//probably an empty/corrupted file if we loop more than 20 times
		f.Seek(x, 2)
		f.Read(buf)
		s = fmt.Sprintf("%s", buf)
		//Don't append any of these ascii characters
		if s != "\n" && s != " " && s != "" && s != "\x00" && s != ":" && s != "h" {
			all = append(all, buf...)
		}
		x--
	}

	//return error if we loop more than 20 times. Normally "h" should be found soon
	err1 := errors.New("Couldn't find the tipnum in .txos file")
	if x <= -20 {
		return 0, err1
	}
	tipstring := fmt.Sprintf("%s", all)
	num, err := strconv.Atoi(reverse(tipstring))
	if err != nil {
		panic(err)
	}
	return num, nil
}

//Builds the offset file
func buildOffsetFile(tip chainhash.Hash, tipnum int, nextMap map[[32]byte]RawHeaderData, offsetfinished chan bool) (int, error) {
	offsetFile, err := os.OpenFile("offsetfile", os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}

	defer offsetFile.Close()
	for fileNum := 0; ; fileNum++ {
		fileName := fmt.Sprintf("blk%05d.dat", fileNum)
		fmt.Printf("Building offsetfile... %s\n", fileName)

		_, err := os.Stat(fileName)
		if os.IsNotExist(err) {
			fmt.Printf("%s doesn't exist; done building\n", fileName)
			break
		}
		rawheaders, err := readRawHeadersFromFile(uint32(fileNum))
		if err != nil {
			panic(err)
		}
		tip, tipnum, err = writeBlockOffset(rawheaders, nextMap, offsetFile, tipnum, tip)
		if err != nil {
			panic(err)
		}
	}
	//write the last height of the offsetfile
	currentOffsetHeightFile, err := os.OpenFile("currentoffsetheight", os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}
	currentOffsetHeightFile.Write(U32tB(uint32(tipnum))[:])
	currentOffsetHeightFile.Close()

	//pass true to let stopParse() know we're finished
	//So it doesn't delete the offsetfile
	offsetfinished <- true
	return tipnum, nil
}

//readRawHeadersFromFile reads only the headers from the given .dat file
func readRawHeadersFromFile(fileNum uint32) ([]RawHeaderData, error) {
	var blockHeaders []RawHeaderData

	fileName := fmt.Sprintf("blk%05d.dat", fileNum)
	f, err := os.Open(fileName)
	if err != nil {
		panic(err)
	}

	fstat, err := f.Stat()
	if err != nil {
		panic(err)
	}

	defer f.Close()
	//skip genesis block
	loc := int64(0)
	//where the block is located from the beginning of the file
	offset := uint32(0)

	//until offset is at the end of the file
	for loc != fstat.Size() {
		b := new(RawHeaderData)
		copy(b.FileNum[:], U32tB(fileNum))
		copy(b.Offset[:], U32tB(offset))

		//check if Bitcoin magic bytes were read
		var magicbytes [4]byte
		f.Read(magicbytes[:])
		if checkMagicByte(magicbytes) == false {
			break
		}

		//read the 4 byte size of the load of the block
		var size [4]byte
		f.Read(size[:])

		//add 8bytes for the magic bytes (4bytes) and size (4bytes)
		offset = offset + LBtU32(size[:]) + uint32(8)

		var blockheader [80]byte
		f.Read(blockheader[:])

		copy(b.Prevhash[:], blockheader[4:32])

		//create block hash
		first := sha256.Sum256(blockheader[:])
		b.CurrentHeaderHash = sha256.Sum256(first[:])

		//offset for the next block from the current position
		loc, err = f.Seek(int64(LBtU32(size[:])) - 80, 1)
		blockHeaders = append(blockHeaders, *b)
		b = nil
	}
	return blockHeaders, nil
}

//Sorts and writes the block offset from the passed in blockHeaders.
func writeBlockOffset(
	blockHeaders []RawHeaderData,//        All headers from the select .dat file
	nextMap map[[32]byte]RawHeaderData,//  Map to save the current block hash 
	offsetFile *os.File,//                 File to save the sorted blocks and locations to
	tipnum int,//                          Current block it's on
	tip chainhash.Hash) (//                Current hash of the block it's on
		chainhash.Hash, int, error) {

	for _, b := range blockHeaders {
		if len(nextMap) > 10000 {//Just a random big number
			fmt.Println("Dead end tip. Exiting...")
			break
		}

		//not in line, add to map
		if b.Prevhash != tip {
			nextMap[b.Prevhash] = b
			continue
		}

		tip = b.CurrentHeaderHash
		tipnum++

		offsetFile.Write(b.FileNum[:])
		offsetFile.Write(b.Offset[:])

		//check for next blocks in map
		stashedBlock, ok := nextMap[tip]
		for ok {
			tip = stashedBlock.CurrentHeaderHash
			tipnum++

			offsetFile.Write(stashedBlock.FileNum[:])
			offsetFile.Write(stashedBlock.Offset[:])
			delete(nextMap, stashedBlock.Prevhash)
			stashedBlock, ok = nextMap[tip]
		}
	}
	return tip, tipnum, nil
}

//readRawBlocksFromFile reads the blocks from the given .dat file and
//returns those blocks.
func getRawBlockFromFile(tipnum int, offsetFile *os.File) (wire.MsgBlock, error) {
	var datFile [4]byte
	var offset [4]byte

	//offset file consists of 8 bytes per block
	//tipnum * 8 gives us the correct position for that block
	offsetFile.Seek(int64(8 * tipnum), 0)

	//Read file and offset for the block
	offsetFile.Read(datFile[:])
	offsetFile.Read(offset[:])

	fileName := fmt.Sprintf("blk%05d.dat", int(BtU32(datFile[:])))
	f, err := os.Open(fileName)
	if err != nil {
		panic(err)
	}
	//+8 skips the 8 bytes of magicbytes and load size
	f.Seek(int64(BtU32(offset[:]) + 8), 0)

	b := new(wire.MsgBlock)
	err = b.Deserialize(f)
	if err != nil {
		panic(err)
	}
	offsetFile.Close()
	f.Close()
	return *b, nil
}

//writeBlock writes to the .txos file.
//Adds - for txinput, - for txoutput, z for unspenable txos, and the height number for that block.
func writeBlock(b wire.MsgBlock, tipnum int, f *os.File,
	batchan chan *leveldb.Batch, wg *sync.WaitGroup) error {

	//s is the string that gets written to testnet.txos
	var s string

	blockBatch := new(leveldb.Batch)

	for blockindex, tx := range b.Transactions {
		for _, in := range tx.TxIn {
			if blockindex > 0 { // skip coinbase "spend"
				opString := in.PreviousOutPoint.String()
				s += "-" + opString + "\n"
				h := HashFromString(opString)
				blockBatch.Put(h[:], U32tB(uint32(tipnum)))
			}
		}

		// creates all txos up to index indicated
		s += "+" + wire.OutPoint{tx.TxHash(), uint32(len(tx.TxOut))}.String()

		for i, out := range tx.TxOut {
			if isUnspendable(out) {
				s += "z" + fmt.Sprintf("%d", i)
			}
		}
		s += "\n"
	}

	//fmt.Printf("--- sending off %d dels at tipnum %d\n", batch.Len(), tipnum)
	wg.Add(1)
	batchan <- blockBatch

	s += fmt.Sprintf("h: %d\n", tipnum)
	_, err := f.WriteString(s)
	if err != nil {
		panic(err)
	}

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

//Converts 4 byte Little Endian slices to uint32.
//Returns ffffffff if something doesn't work.
func LBtU32(b []byte) uint32 {
        if len(b) != 4 {
                fmt.Printf("Got %x to LBtU32 (%d bytes)\n", b, len(b))
                return 0xffffffff
        }
        var i uint32
        buf := bytes.NewBuffer(b)
        binary.Read(buf, binary.LittleEndian, &i)
        return i
}

//checkMagicByte checks for the Bitcoin magic bytes.
//returns false if it didn't read the Bitcoin magic bytes.
func checkMagicByte(bytesgiven [4]byte) bool {
	if bytesgiven != [4]byte{0x0b, 0x11, 0x09, 0x07} && //testnet
		bytesgiven != [4]byte{0xf9, 0xbe, 0xb4, 0xd9} { // mainnet
		fmt.Printf("got non magic bytes %x, finishing\n", bytesgiven)
		return false
	} else {
		return true
	}
}

//StopParse receives and handles sig from the system.
//Handles SIGTERM, SIGINT, and SIGQUIT.
func stopParse(sig chan bool, offsetfinished chan bool) {
	<-sig
	select {
	//If offsetfile is there or was built, don't remove it
	case <-offsetfinished:
		os.Exit(1)
	//If nothing is received, delete offsetfile and currentoffsetheight
	default:
		os.Remove("offsetfile")
		os.Remove("currentoffsetheight")
		fmt.Println("offsetfile incomplete, removing...")
	}
	fmt.Println("Exiting...")
	os.Exit(1)
}

//Reverses the given string.
//"asdf" becomes "fdsa".
func reverse(s string) (result string) {
	for _,v := range s {
		result = string(v) + result
	}
	return
}

//hasAccess reports whether we have acces to the named file.
//Does NOT tell us if the file exists or not.
//File might exist but may not be available to us
func hasAccess(fileName string) bool {
	if _, err := os.Stat(fileName); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}
