package csn

import (
	"fmt"
	"os"
	"time"

	"github.com/btcsuite/btcd/wire"
	"github.com/mit-dci/utreexo/cmd/util"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// run IBD from block proof data
// we get the new utxo info from the same txos text file
func IBDClient(net wire.BitcoinNet,
	offsetfile string, ttldb string, sig chan bool) error {

	//Channel to alert the main loop to break
	stopGoing := make(chan bool, 1)

	//Channel to alert stopTxottl it's ok to exit
	done := make(chan bool, 1)

	go stopRunIBD(sig, stopGoing, done)

	// Check if the blk*.dat file given is a testnet/mainnet/regtest file corresponding to net
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

	util.MakePaths()

	p, height, lastIndexOffsetHeight, err := initCSNState()
	if err != nil {
		panic(err)
	}

	lookahead := int32(1000) // keep txos that last less than this many blocks
	totalTXOAdded := 0
	totalDels := 0

	//bool for stopping the scanner.Scan loop
	var stop bool

	// To send/receive blocks from blockreader()
	txChan := make(chan util.TxToWrite, 10)

	// Reads blocks asynchronously from blk*.dat files
	go util.BlockReader(txChan,
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

	for ; height != lastIndexOffsetHeight && stop != true; height++ {

		txs := <-txChan

		err = genPollard(txs.Txs, txs.Height, &totalTXOAdded,
			lookahead, &totalDels, plustime, pFile, pOffsetFile, lvdb, &p)
		if err != nil {
			panic(err)
		}

		//if height%10000 == 0 {
		//	fmt.Printf("Block %d %s plus %.2f total %.2f proofnodes %d \n",
		//		height, newForest.Stats(),
		//		plustime.Seconds(), time.Now().Sub(starttime).Seconds(),
		//		totalProofNodes)
		//}

		if height%10000 == 0 {
			fmt.Printf("Block %d add %d del %d %s plus %.2f total %.2f \n",
				height+1, totalTXOAdded, totalDels, p.Stats(),
				plustime.Seconds(), time.Now().Sub(starttime).Seconds())
		}
		/*
			if height%100000 == 0 {
				fmt.Printf(MemStatString(fname))
			}
		*/

		//Check if stopSig is no longer false
		//stop = true makes the loop exit
		select {
		case stop = <-stopGoing:
		default:
		}
	}
	pFile.Close()
	pOffsetFile.Close()

	fmt.Printf("Block %d add %d del %d %s plus %.2f total %.2f \n",
		height, totalTXOAdded, totalDels, p.Stats(),
		plustime.Seconds(), time.Now().Sub(starttime).Seconds())

	saveIBDsimData(height, p)

	fmt.Println("Done Writing")

	done <- true

	return nil
}
