package csn

import (
	"fmt"
	"time"

	"github.com/mit-dci/utreexo/util"
)

// run IBD from block proof data
// we get the new utxo info from the same txos text file
func IBD(sig chan bool) {

	// Channel to alert the main loop to break when receiving a quit signal from
	// the OS
	haltRequest := make(chan bool, 1)

	// Channel to alert stopRunIBD it's ok to exit
	// Makes it wait for flushing to disk
	haltAccept := make(chan bool, 1)

	go stopRunIBD(sig, haltRequest, haltAccept)

	p, height, err := initCSNState()
	if err != nil {
		panic(err)
	}

	// caching parameter. Keeps txos that are spent before than this many blocks
	lookahead := int32(1000)

	// for benchmarking
	var totalTXOAdded, totalDels int

	// blocks come in and sit in the blockQueue
	// They should come in from the network -- right now they're coming from the
	// disk but it should be the exact same thing
	ublockQueue := make(chan util.UBlock, 10)

	// Reads blocks asynchronously from blk*.dat files, and the proof.dat, and DB
	// this will be a network reader, with the server sending the same stuff over
	go util.UblockNetworkReader(
		ublockQueue, "127.0.0.1:8338", height, lookahead)

	var plustime time.Duration
	starttime := time.Now()

	// bool for stopping the below for loop
	var stop bool
	for ; !stop; height++ {

		blocknproof := <-ublockQueue

		err = putBlockInPollard(blocknproof,
			&totalTXOAdded, &totalDels, plustime, &p)
		if err != nil {
			panic(err)
		}

		if height%10000 == 0 {
			fmt.Printf("Block %d add %d del %d %s plus %.2f total %.2f \n",
				height, totalTXOAdded, totalDels, p.Stats(),
				plustime.Seconds(), time.Now().Sub(starttime).Seconds())
		}

		// Check if stopSig is no longer false
		// stop = true makes the loop exit
		select {
		case stop = <-haltRequest:
		default:
		}

	}
	fmt.Printf("Block %d add %d del %d %s plus %.2f total %.2f \n",
		height, totalTXOAdded, totalDels, p.Stats(),
		plustime.Seconds(), time.Now().Sub(starttime).Seconds())

	saveIBDsimData(height, p)

	fmt.Println("Done Writing")

	haltAccept <- true
}
