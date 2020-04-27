package csn

import (
	"fmt"
	"os"
	"time"

	"github.com/btcsuite/btcd/wire"
)

func RunIBD(net wire.BitcoinNet, offsetfile string, ttldb string, sig chan bool) error {
	// start server & listen
	// go IBDServer()

	// start client & connect
	return IBDClient(net, offsetfile, ttldb, sig)
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
