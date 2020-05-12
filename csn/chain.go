package csn

import (
	"fmt"
	"os"

	"github.com/mit-dci/utreexo/accumulator"
	"github.com/mit-dci/utreexo/util"
)

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
