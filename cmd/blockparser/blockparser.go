package blockparser

import (
	"crypto/sha256"
	"fmt"
	"os"
	"sync"

	"github.com/mit-dci/lit/btcutil/chaincfg/chainhash"
	"github.com/mit-dci/lit/wire"
	"github.com/mit-dci/utreexo/cmd/utils"
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
	CurrentHeaderHash [32]byte
	Prevhash          [32]byte
	FileNum           [4]byte
	Offset            [4]byte
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
func Parser(isTestnet bool, ttldb string, offsetfile string, sig chan bool) error {

	//Sometimes defer lvdb.Close() will not work so these channels
	//are to break out of the main loop and wait for the waitgroup
	//so leveldb can close gracefully if SIGINT, SIGTERM, SIGQUIT is given.

	//Channel to alert stopParse() that buildOffsetFile() has been finished
	offsetfinished := make(chan bool, 1)

	//Channel to alert the main loop to break
	stopGoing := make(chan bool, 1)

	//Tell stopParse that the main loop is running
	running := make(chan bool, 1)

	//Tell stopParse that the main loop is ok to exit now
	done := make(chan bool, 1)
	//listen for SIGINT, SIGTERM, or SIGQUIT from the os
	go stopParse(sig, offsetfinished, stopGoing, running, done, offsetfile)
	//defaults to the testnet Gensis tip
	tip := testNet3GenHash

	//If given the option testnet=true, check if the blk00000.dat file
	//in the directory is a testnet file. Vise-versa for mainnet
	simutil.CheckTestnet(isTestnet)

	var currentOffsetHeight int
	tipnum := 0
	nextMap := make(map[[32]byte]RawHeaderData)

	//if there isn't an offset file, make one
	if simutil.HasAccess(offsetfile) == false {
		currentOffsetHeight, _ = buildOffsetFile(tip, tipnum, nextMap, offsetfile, offsetfinished)
	} else {
		//if there is a offset file, we should pass true to offsetfinished
		//to let stopParse() know that it shouldn't delete offsetfile
		offsetfinished <- true
	}
	//if there is a .txos file, get the tipnum from that
	var t [4]byte
	if simutil.HasAccess("tipfile") {
		tf, err := os.Open("tipfile")
		if err != nil {
			panic(err)
		}
		tf.Read(t[:])
		tipnum = int(simutil.BtU32(t[:]))
	}

	tipFile, err := os.OpenFile("tipfile", os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}

	//grab the last block height from currentoffsetheight
	//currentoffsetheight saves the last height from the offsetfile
	var currentOffsetHeightByte [4]byte
	currentOffsetHeightFile, err := os.Open("currentoffsetheight")
	if err != nil {
		panic(err)
	}
	currentOffsetHeightFile.Read(currentOffsetHeightByte[:])
	currentOffsetHeight = int(simutil.BtU32(currentOffsetHeightByte[:]))

	o := new(opt.Options)
	o.CompactionTableSizeMultiplier = 8
	lvdb, err := leveldb.OpenFile(ttldb, o)
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

	fmt.Println("Building ttldb...")
	fmt.Println("Starting from block:", tipnum)

	//bool to stop the main loop
	var stop bool

	//tell stopParse that the main loop is running
	running <- true

	offsetFile, err := os.Open(offsetfile)
	if err != nil {
		panic(err)
	}
	//read off the offset file and start writing to the .txos file
	for ; tipnum != currentOffsetHeight && stop != true; tipnum++ {

		block, err := simutil.GetRawBlockFromFile(tipnum, offsetFile)
		if err != nil {
			panic(err)
		}

		//write to the .txos file
		writeBlock(block, tipnum+1, tipFile, batchan, &batchwg) //tipnum is +1 since we're skipping the genesis block

		//Just something to let the user know that the program is still running
		//The actual block the program is on is +1 of the printed number
		if tipnum%50000 == 0 {
			fmt.Println("On block :", tipnum)
		}
		select {
		case stop = <-stopGoing:
		default:
		}
	}
	batchwg.Wait()
	fmt.Println("Finished writing.")

	//tell stopParse that it's ok to exit
	done <- true
	return nil
}

//Builds the offset file
func buildOffsetFile(tip chainhash.Hash, tipnum int, nextMap map[[32]byte]RawHeaderData, offsetfile string, offsetfinished chan bool) (int, error) {
	offsetFile, err := os.OpenFile(offsetfile, os.O_CREATE|os.O_WRONLY, 0600)
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
	currentOffsetHeightFile.Write(simutil.U32tB(uint32(tipnum))[:])
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
		copy(b.FileNum[:], simutil.U32tB(fileNum))
		copy(b.Offset[:], simutil.U32tB(offset))

		//check if Bitcoin magic bytes were read
		var magicbytes [4]byte
		f.Read(magicbytes[:])
		if simutil.CheckMagicByte(magicbytes) == false {
			break
		}

		//read the 4 byte size of the load of the block
		var size [4]byte
		f.Read(size[:])

		//add 8bytes for the magic bytes (4bytes) and size (4bytes)
		offset = offset + simutil.LBtU32(size[:]) + uint32(8)

		var blockheader [80]byte
		f.Read(blockheader[:])

		copy(b.Prevhash[:], blockheader[4:32])

		//create block hash
		first := sha256.Sum256(blockheader[:])
		b.CurrentHeaderHash = sha256.Sum256(first[:])

		//offset for the next block from the current position
		loc, err = f.Seek(int64(simutil.LBtU32(size[:]))-80, 1)
		blockHeaders = append(blockHeaders, *b)
		b = nil
	}
	return blockHeaders, nil
}

//Sorts and writes the block offset from the passed in blockHeaders.
func writeBlockOffset(
	blockHeaders []RawHeaderData, //        All headers from the select .dat file
	nextMap map[[32]byte]RawHeaderData, //  Map to save the current block hash
	offsetFile *os.File, //                 File to save the sorted blocks and locations to
	tipnum int, //                          Current block it's on
	tip chainhash.Hash) ( //                Current hash of the block it's on
	chainhash.Hash, int, error) {

	for _, b := range blockHeaders {
		if len(nextMap) > 10000 { //Just a random big number
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

//writeBlock writes to the .txos file.
//Adds - for txinput, - for txoutput, z for unspenable txos, and the height number for that block.
func writeBlock(b wire.MsgBlock, tipnum int, f *os.File,
	batchan chan *leveldb.Batch, wg *sync.WaitGroup) error {

	//s is the string that gets written to .txos
	//var s string

	blockBatch := new(leveldb.Batch)

	for blockindex, tx := range b.Transactions {
		for _, in := range tx.TxIn {
			if blockindex > 0 { // skip coinbase "spend"
				//hashing because blockbatch wants a byte slice
				opString := in.PreviousOutPoint.String()
				h := simutil.HashFromString(opString)
				//s += "-" + opString + "\n"
				//fmt.Println("just txin:", in.PreviousOutPoint)
				//fmt.Println("hight:", tipnum)
				blockBatch.Put(h[:], simutil.U32tB(uint32(tipnum)))
			}
		}

		// creates all txos up to index indicated
		//s += "+" + wire.OutPoint{tx.TxHash(), uint32(len(tx.TxOut))}.String()

		//for i, out := range tx.TxOut {
		//	if isUnspendable(out) {
		//		s += "z" + fmt.Sprintf("%d", i)
		//	}
		//}
		//s += "\n"
	}

	//fmt.Printf("--- sending off %d dels at tipnum %d\n", batch.Len(), tipnum)
	wg.Add(1)
	//sent to dbworker to be written to ttldb asynchronously
	batchan <- blockBatch

	//s += fmt.Sprintf("h: %d\n", tipnum)
	//write to the .txos file
	//_, err := f.WriteAt(simutil.U32tB(uint32(tipnum))[:], 0)
	_, err := f.WriteAt(simutil.U32tB(uint32(tipnum)), 0)
	if err != nil {
		panic(err)
	}

	return nil
}

// dbWorker writes everything to the db. It's it's own goroutine so it
// can work at the same time that the reads are happening
func dbWorker(
	bChan chan *leveldb.Batch, lvdb *leveldb.DB, wg *sync.WaitGroup) {

	for {
		b := <-bChan
		//		fmt.Printf("--- writing batch %d dels\n", b.Len())
		err := lvdb.Write(b, nil)
		if err != nil {
			fmt.Println(err.Error())
		}
		//		fmt.Printf("wrote %d deletions to leveldb\n", b.Len())
		wg.Done()
	}
}

//StopParse receives and handles sig from the system.
//Handles SIGTERM, SIGINT, and SIGQUIT.
func stopParse(sig chan bool, offsetfinished chan bool, stopGoing chan bool, running chan bool, done chan bool, offsetfile string) {
	<-sig
	stopGoing <- true
	select {
	//If offsetfile is there or was built, don't remove it
	case <-offsetfinished:
		select {
		case <-running:
			<-done
		default:
		}
	//If nothing is received, delete offsetfile and currentoffsetheight
	default:
		select {
		case <-running:
			<-done
		default:
			os.Remove(offsetfile)
			os.Remove("currentoffsetheight")
			fmt.Println("offsetfile incomplete, removing...")
		}
	}

	fmt.Println("Exiting...")
	os.Exit(0)
}
