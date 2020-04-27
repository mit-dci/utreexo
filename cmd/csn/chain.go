package csn

import (
	"fmt"
	"os"

	"github.com/mit-dci/utreexo/accumulator"
	"github.com/mit-dci/utreexo/cmd/util"
)

// initCSNState attempts to load and initialize the CSN state from the disk.
// If a CSN state is not present, chain is initialized to the genesis
func initCSNState() (
	p accumulator.Pollard, height int32, lastIndexOffsetHeight int32, err error) {

	var offsetInitialized, pollardInitialized bool

	// bool to check if the offsetfile is present
	offsetInitialized = util.HasAccess(util.OffsetFilePath)

	// We expect the offsetdata to be present
	// TODO this will be depreciated in the future
	if offsetInitialized {
		var err error
		lastIndexOffsetHeight, err = restoreLastIndexOffsetHeight()
		if err != nil {
			return p, 0, 0, err
		}
	} else {
		return p, 0, 0, fmt.Errorf("No offsetdata present. " +
			"Please run `genproofs` first and try again")
	}

	// bool to check if the pollarddata is present
	pollardInitialized = util.HasAccess(util.PollardFilePath)

	if pollardInitialized {
		fmt.Println("Has access to forestdata, resuming")
		var err error
		p, err = restorePollard()
		if err != nil {
			return p, 0, 0, err
		}
		height, err = restorePollardHeight()
		if err != nil {
			return p, 0, 0, err
		}

	} else {
		fmt.Println("Creating new pollarddata")

		// Create files needed for pollard
		_, err := os.OpenFile(
			util.PollardHeightFilePath, os.O_CREATE, 0600)
		if err != nil {
			return p, height, lastIndexOffsetHeight, err
		}
		_, err = os.OpenFile(
			util.PollardFilePath, os.O_CREATE, 0600)
		if err != nil {
			return p, height, lastIndexOffsetHeight, err
		}
	}

	return
}
