package txottl

import (
	"fmt"
	"os"
	//"time"

	"github.com/mit-dci/lit/wire"
	"github.com/mit-dci/utreexo/cmd/utils"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// for parallel txofile building we need to have a buffer
type txotx struct {
	// the inputs as a string.  Note that this string has \ns in it, and it's
	// the whole block of inputs.
	// also includes the whole + line with the output!
	// includes the +, the txid the ; z and ints, but no x, ints or commas
	// (basically whatever lit produced in mainnet.txos)
	txText string

	outputtxid string
	// here's all the death heights of the output txos
	deathHeights []uint32
}

type deathInfo struct {
	deathHeight, blockPos, txPos uint32
}

// for each block, make a slice of txotxs in order.  The slice will stay in order.
// also make the deathheights slices for all the txotxs the right size.
// then hand the []txotx slice over to the worker function which can make the
// lookups in parallel and populate the deathheights.  From there you can go
// back to serial to write back to the txofile.

// ttlLookup takes the slice of txotxs and fills in the deathheights
func lookupBlock(block []*txotx, db *leveldb.DB) {

	// I don't think buffering this will do anything..?
	infoChan := make(chan deathInfo)

	var remaining uint32

	// go through every tx
	for blockPos, tx := range block {
		// go through every output
		for txPos, _ := range tx.deathHeights {
			// increment counter, and send off to a worker
			remaining++
			go lookerUpperWorker(
				tx.outputtxid, uint32(blockPos), uint32(txPos), infoChan, db)
		}
	}

	var rcv deathInfo
	for remaining > 0 {
		//		fmt.Printf("%d left\t", remaining)
		rcv = <-infoChan
		block[rcv.blockPos].deathHeights[rcv.txPos] = rcv.deathHeight
		remaining--
	}

	return
}

// lookerUpperWorker does the hashing and db read, then returns it's result
// via a channel
func lookerUpperWorker(
	txid string, blockPos, txPos uint32,
	infoChan chan deathInfo, db *leveldb.DB) {

	// start deathInfo struct to send back
	var di deathInfo
	di.blockPos, di.txPos = blockPos, txPos

	// build string and hash it (nice that this in parallel too)
	utxostring := fmt.Sprintf("%s;%d", txid, txPos)
	opHash := simutil.HashFromString(utxostring)

	// make DB lookup
	ttlbytes, err := db.Get(opHash[:], nil)
	if err == leveldb.ErrNotFound {
		//		fmt.Printf("can't find %s;%d in file", txid, txPos)
		ttlbytes = make([]byte, 4) // not found is 0
	} else if err != nil {
		// some other error
		panic(err)
	}
	if len(ttlbytes) != 4 {
		fmt.Printf("val len %d, op %s;%d\n", len(ttlbytes), txid, txPos)
		panic("ded")
	}

	di.deathHeight = simutil.BtU32(ttlbytes)
	// send back to the channel and this output is done
	infoChan <- di

	return
}

// read from the DB and tack on TTL values
func ReadTTLdb(isTestnet bool, txos string, ttldb string, offsetfile string, sig chan bool) error {

	//Channel to alert the main loop to break
	stopGoing := make(chan bool, 1)

	//Channel to alert stopTxottl it's ok to exit
	done := make(chan bool, 1)

	//Handles SIG from the os
	go stopTxottl(sig, stopGoing, done)

	//Check if -testnet=true is given and that the actual file
	//is for testnet and vise versa
	simutil.CheckTestnet(isTestnet)

	// open database
	o := new(opt.Options)
	o.CompactionTableSizeMultiplier = 8
	o.ReadOnly = true
	lvdb, err := leveldb.OpenFile(ttldb, o)
	if err != nil {
		panic(err)
	}
	defer lvdb.Close()

	//Make a ttl.*.txos file. Append if it exists
	ttlfile, err := os.OpenFile("ttl."+txos, os.O_APPEND|os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		panic(err)
	}
	defer ttlfile.Close()

	var tipnum int
	//Get the tip number from the ttl.*.txos file
	//Returns 0 if there isn't a ttl.*.txos file
	if simutil.HasAccess("ttl." + txos) {
		tipnum, err = simutil.GetTipNum("ttl." + txos)
		if err != nil {
			panic(err)
		}
	}

	var currentOffsetHeight int
	//grab the last block height from currentoffsetheight
	//currentoffsetheight saves the last height from the offsetfile
	var currentOffsetHeightByte [4]byte
	currentOffsetHeightFile, err := os.Open("currentoffsetheight")
	if err != nil {
		panic(err)
	}
	currentOffsetHeightFile.Read(currentOffsetHeightByte[:])
	currentOffsetHeight = int(simutil.BtU32(currentOffsetHeightByte[:]))

	//allocate txotx to the heap and point to that
	blocktxs := []*txotx{new(txotx)}

	//bool for stopping the scanner.Scan loop
	var stop bool

	offsetFile, err := os.Open(offsetfile)
	if err != nil {
		panic(err)
	}
	fmt.Println("Generating txo time to live...")

	//stop only becomes true when the os gives SIGINT, SIGTERM, SIGQUIT
	//AND the block that it was working on is written
	for ; tipnum != currentOffsetHeight && stop != true; tipnum++ {

		//rawblocktime := time.Now()
		block, err := simutil.GetRawBlockFromFile(tipnum, offsetFile)
		if err != nil {
			panic(err)
		}
		//donerawblocktime := time.Now()
		//fmt.Println("rawblock took:", donerawblocktime.Sub(rawblocktime))
		//writetxostime := time.Now()
		//write to the .txos file
		err = writeTxos(block, blocktxs, tipnum+1, ttlfile, lvdb) //tipnum is +1 since we're skipping the genesis block
		if err != nil {
			panic(err)
		}
		//donewritetxostime := time.Now()
		//fmt.Println("writeTxos took:", donewritetxostime.Sub(writetxostime))

		//Just something to let the user know that the program is still running
		//The actual block the program is on is +1 of the printed number
		if tipnum%100 == 0 {
			fmt.Println("On block :", tipnum)
		}

		// start next block
		// wipe all block txs
		blocktxs = []*txotx{new(txotx)}

		//Check if stopSig is no longer false
		//stop = true makes the loop exit
		select {
		case stop = <-stopGoing:
		default:
		}

	}
	fmt.Println("Done Writing.")

	//Tell stopTxottl that it's ok to quit now
	done <- true
	return nil
}

//writeTxos writes to the .txos file.
//Adds '+' for txinput, '-' for txoutput, 'z' for unspenable txos, and the height number for that block.
func writeTxos(tx []*wire.MsgTx, blocktxs []*txotx, tipnum int,
	ttlfile *os.File, lvdb *leveldb.DB) error {

	//s is the string that gets written to .txos
	var s string

	for blockindex, tx := range tx {
		for _, in := range tx.TxIn {
			if blockindex > 0 { // skip coinbase "spend"
				//hashing because blockbatch wants a byte slice
				opString := in.PreviousOutPoint.String()
				blocktxs[len(blocktxs)-1].txText += "-" + opString + "\n"
			}
		}

		//creates all txos up to index indicated
		txhash := tx.TxHash()
		numoutputs := uint32(len(tx.TxOut))
		blocktxs[len(blocktxs)-1].txText += "+" + wire.OutPoint{txhash, numoutputs}.String()

		//Adds z and index for all OP_RETURN transactions
		//We don't keep track of the OP_RETURNS so probably can get rid of this
		for i, out := range tx.TxOut {
			if simutil.IsUnspendable(out) {
				blocktxs[len(blocktxs)-1].txText += "z" + fmt.Sprintf("%d", i)
			}
		}
		blocktxs[len(blocktxs)-1].txText += "x"
		//txid := tx.TxHash().String()
		blocktxs[len(blocktxs)-1].outputtxid = txhash.String()
		blocktxs[len(blocktxs)-1].deathHeights = make([]uint32, numoutputs)

		// done with this txotx, make the next one and append
		blocktxs = append(blocktxs, new(txotx))
	}

	//TODO Get rid of this. This eats up cpu
	//we started a tx but shouldn't have
	blocktxs = blocktxs[:len(blocktxs)-1]

	// call function to make all the db lookups and find deathheights
	lookupBlock(blocktxs, lvdb)

	// write filled in txotx slice
	for _, tx := range blocktxs {
		// the txTest has all the inputs, and the output, and an x.
		// we just have to stick the numbers and commas on here.
		txstring := tx.txText
		for _, deathheight := range tx.deathHeights {
			if deathheight == 0 {
				txstring += "s,"
			} else {
				txstring += fmt.Sprintf("%d,", int(deathheight)-tipnum)
			}
		}

		//add to s to be written to .txos file
		s += fmt.Sprintf(txstring + "\n")
	}

	s += fmt.Sprintf("h: %d\n", tipnum)

	//write to the .txos file
	_, err := ttlfile.WriteString(s)
	if err != nil {
		return err
	}

	return nil
}

//stopTxottl receives and handles sig from the system
//Handles SIGTERM, SIGINT, and SIGQUIT
func stopTxottl(sig chan bool, stopGoing chan bool, done chan bool) {
	<-sig
	//Tell ReadTTLdb to finish the block it's working on
	stopGoing <- true

	//Wait until ReadTTLdb says it's ok to quit
	<-done
	fmt.Println("Exiting...")
	os.Exit(0)
}
