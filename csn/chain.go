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
	p accumulator.Pollard, height int32, knownTipHeight int32, err error) {

	var offsetInitialized, pollardInitialized bool

	// bool to check if the offsetfile is present
	offsetInitialized = util.HasAccess(util.OffsetFilePath)

	// We expect the offsetdata to be present
	// TODO this will be depreciated in the future
	if offsetInitialized {
		var info os.FileInfo
		info, err = os.Stat(util.POffsetFilePath)
		if err != nil {
			return
		}
		if info.Size()%8 != 0 {
			err = fmt.Errorf("offsetfile %d bytes, not multiple of 8",
				info.Size())
			return
		}
		knownTipHeight = int32(info.Size() / 8)
	} else {
		err = fmt.Errorf("No offsetdata present. " +
			"Please run `genproofs` first and try again")
		return
	}

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
