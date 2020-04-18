package csn

import (
	"os"
	"time"

	"github.com/btcsuite/btcd/wire"
	"github.com/mit-dci/utreexo/log"
	"github.com/mit-dci/utreexo/cmd/util"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// run IBD from block proof data
// we get the new utxo info from the same txos text file
func IBDClient(net wire.BitcoinNet,
	offsetfile string, ttldb string, sig chan bool, loggers log.Loggers) error {
	log := loggers.Csn

	// Channel to alert the main loop to break when receiving a quit signal from
	// the OS
	stopGoing := make(chan bool, 1)

	// Channel to alert stopRunIBD it's ok to exit
	// Makes it wait for flushing to disk
	done := make(chan bool, 1)

	go stopRunIBD(sig, stopGoing, done)

	// Check if the blk*.dat file given is a testnet/mainnet/regtest
	// file corresponding to net
	util.CheckNet(net)

	// open database
	o := new(opt.Options)
	o.CompactionTableSizeMultiplier = 8
	o.ReadOnly = true
	lvdb, err := leveldb.OpenFile(ttldb, o)
	if err != nil {
		panic(err)
	}
	defer lvdb.Close()

	// Make neccesary directories
	util.MakePaths()

	p, height, lastIndexOffsetHeight, err := initCSNState(loggers)
	if err != nil {
		panic(err)
	}

	// caching parameter. Keeps txos that are spent before than this many blocks
	lookahead := int32(1000)

	// for benchmarking
	var totalTXOAdded, totalDels int

	// To send/receive blocks from blockreader()
	blockChan := make(chan util.BlockToWrite, 10)

	// Reads blocks asynchronously from blk*.dat files
	go util.BlockReader(blockChan,
		lastIndexOffsetHeight, height, util.OffsetFilePath)

	pFile, err := os.OpenFile(
		util.PFilePath, os.O_RDONLY, 0400)
	if err != nil {
		return err
	}

	pOffsetFile, err := os.OpenFile(
		util.POffsetFilePath, os.O_RDONLY, 0400)
	if err != nil {
		return err
	}

	var plustime time.Duration
	starttime := time.Now()

	// bool for stopping the below for loop
	var stop bool
	for ; height != lastIndexOffsetHeight && stop != true; height++ {

		txs := <-blockChan

		err = genPollard(txs.Txs, txs.Height, &totalTXOAdded, &totalDels,
			lookahead, plustime, pFile, pOffsetFile, lvdb, &p)
		if err != nil {
			panic(err)
		}

		//if height%10000 == 0 {
		//	log.Printf("Block %d %s plus %.2f total %.2f proofnodes %d \n",
		//		height, newForest.Stats(),
		//		plustime.Seconds(), time.Now().Sub(starttime).Seconds(),
		//		totalProofNodes)
		//}

		if height%10000 == 0 {
			log.Printf("Block %d add %d del %d %s plus %.2f total %.2f \n",
				height+1, totalTXOAdded, totalDels, p.Stats(),
				plustime.Seconds(), time.Now().Sub(starttime).Seconds())
		}

		// Check if stopSig is no longer false
		// stop = true makes the loop exit
		select {
		case stop = <-stopGoing:
		default:
		}
	}
	pFile.Close()
	pOffsetFile.Close()

	log.Printf("Block %d add %d del %d %s plus %.2f total %.2f \n",
		height, totalTXOAdded, totalDels, p.Stats(),
		plustime.Seconds(), time.Now().Sub(starttime).Seconds())

	saveIBDsimData(height, p)

	log.Println("Done Writing")

	done <- true

	return nil
}
