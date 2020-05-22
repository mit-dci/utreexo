package csn

import (
	"fmt"
	"os"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
	"github.com/mit-dci/utreexo/accumulator"
	"github.com/mit-dci/utreexo/util"
)

func RunIBD(p *chaincfg.Params, sig chan bool) error {

	pol, h, err := initCSNState()
	if err != nil {
		return err
	}
	c := new(Csn)
	c.pollard = pol

	c.Start(h, "127.0.0.1:8338", "compactstate", "", p)
	// start client & connect
	go IBD(sig)

	return nil
}

// Start starts up a compact state node, and returns channels for txs and
// block heights.
func (c *Csn) Start(height int32, host, path, proxyURL string,
	params *chaincfg.Params) (chan wire.MsgTx, chan int32, error) {

	// p, height, err := initCSNState()
	// if err != nil {
	// 	panic(err)
	// }

	return nil, nil, nil
}

// initCSNState attempts to load and initialize the CSN state from the disk.
// If a CSN state is not present, chain is initialized to the genesis
func initCSNState() (
	p accumulator.Pollard, height int32, err error) {

	var pollardInitialized bool

	// bool to check if the pollarddata is present
	pollardInitialized = util.HasAccess(util.PollardFilePath)

	if pollardInitialized {
		fmt.Println("Has access to forestdata, resuming")
		p, err = restorePollard()
		if err != nil {
			return
		}
		height, err = restorePollardHeight()
		if err != nil {
			return
		}

	} else {
		fmt.Println("Creating new pollarddata")
		// start at height 1
		height = 1
		// Create files needed for pollard
		_, err = os.OpenFile(
			util.PollardHeightFilePath, os.O_CREATE, 0600)
		if err != nil {
			return
		}
		_, err = os.OpenFile(
			util.PollardFilePath, os.O_CREATE, 0600)
		if err != nil {
			return
		}
	}

	return
}

func stopRunIBD(sig chan bool, stopGoing chan bool, done chan bool) {
	// Listen for SIGINT, SIGTERM, and SIGQUIT from the user
	<-sig

	// Sometimes there are bugs that make the program run forver.
	// Utreexo binary should never take more than 10 seconds to exit
	go func() {
		time.Sleep(10 * time.Second)
		fmt.Println("Program timed out. Force quitting." +
			"Data likely corrupted")
		os.Exit(1)
	}()

	// Tell the user that the sig is received
	fmt.Println("User exit signal received. Exiting...")

	// Tell Runibd() to finish the block it's working on
	stopGoing <- true

	// Wait until RunIBD() says it's ok to quit
	<-done
	os.Exit(0)
}
