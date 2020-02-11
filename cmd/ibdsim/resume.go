package ibdsim

import (
	"fmt"
	"os"

	"github.com/mit-dci/utreexo/cmd/simutil"
	"github.com/mit-dci/utreexo/utreexo"
)

// InitGenProofs restores the variables from disk to memory.
// Returns the height, currentoffsetheight, and pOffset genproofs left at.
//
// If starting anew, pOffset and height is set to 0, forest will be initalized.
// currentOffsetHeight will be set to whatever block height is last in the
// blk*.dat files provided.
func InitGenProofs(isTestnet bool, offsetInitialized bool,
	forestInitialized bool, offsetfinished chan bool) (
	int32, int32, uint32, *utreexo.Forest) {

	currentOffsetHeight, height, err := initOffsetData(
		isTestnet, offsetInitialized, offsetfinished)
	if err != nil {
		panic(err)
	}

	pOffset := restoreLastProofFileOffset()

	forest := initForest(forestInitialized)

	return height, currentOffsetHeight, pOffset, forest
}

// InitIBDsim restores the variables from disk to memory.
// Returns the currentOffsetHeight, height and pollard.
//
// If started anew, height is set to 0 and pollard will be empty.
// currentOffsetHeight is always based off the offsetdata generated
// with genproofs.
func InitIBDsim(pollardInitialized bool) (int32, int32, utreexo.Pollard) {

	var currentOffsetHeight, height int32

	currentOffsetHeight = restoreCurrentOffset()

	// only restore height if pollard has already been initialized
	if pollardInitialized == true {
		height = restorePollardHeight()
	}

	pollard, err := initPollard(pollardInitialized)
	if err != nil {
		panic(err)
	}
	return currentOffsetHeight, height, pollard
}

// initPollard restores the pollard from disk to memory.
// If starting anew, it just returns a empty pollard.
func initPollard(pollardInitialized bool) (utreexo.Pollard, error) {

	var pollard utreexo.Pollard

	if pollardInitialized == true {
		fmt.Println("pollardfile access")

		// Restore Pollard
		pollardFile, err := os.OpenFile(
			simutil.PollardFilePath, os.O_RDWR, 0600)
		if err != nil {
			panic(err)
		}
		err = pollard.RestorePollard(pollardFile)
		if err != nil {
			panic(err)
		}
	} else {
		// create a file for the height to be stored at
		_, err := os.OpenFile(
			simutil.PollardHeightFilePath, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			panic(err)
		}

	}

	return pollard, nil
}

// initForest initializes forest. If forestdata directory exists, it will
// initialize/resume based on that data.
func initForest(forestInitialized bool) *utreexo.Forest {

	var forest *utreexo.Forest
	if forestInitialized == true {
		fmt.Println("Has access to forestfile, resuming...")

		// Where the forestfile exists
		forestFile, err := os.OpenFile(
			simutil.ForestFilePath, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			panic(err)
		}

		// Other forest variables
		miscForestFile, err := os.OpenFile(
			simutil.MiscForestFilePath, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			panic(err)
		}

		// Restores all the forest data
		forest, err = utreexo.RestoreForest(miscForestFile, forestFile)
		if err != nil {
			panic(err)
		}
	} else {
		// Where the forestfile exists
		forestFile, err := os.OpenFile(
			simutil.ForestFilePath, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			panic(err)
		}

		fmt.Println("No forestFile access")
		forest = utreexo.NewForest(forestFile)
	}

	return forest
}

// restorePollardHeight restores the current height that pollard is synced to
// Not to be confused with the height variable for genproofs
func restorePollardHeight() int32 {

	// Restore height
	pHeightFile, err := os.OpenFile(
		simutil.PollardHeightFilePath, os.O_RDONLY, 0600)
	if err != nil {
		panic(err)
	}
	var t [4]byte
	_, err = pHeightFile.Read(t[:])
	if err != nil {
		panic(err)
	}
	height := simutil.BtI32(t[:])

	return height
}

// initOffsetData creates or restores the offsetfile needed to index the
// blocks in the raw blk*.dat files.
func initOffsetData(
	isTestnet bool, offsetInitialized bool, offsetfinished chan bool) (
	int32, int32, error) {

	var currentOffsetHeight, height int32

	// if there isn't an offsetfile
	if offsetInitialized == false {

		var tip simutil.Hash
		if isTestnet == true {
			tip = simutil.TestNet3GenHash
		} else {
			tip = simutil.MainnetGenHash
		}

		fmt.Println("offsetfile not present. Building...")
		var err error
		currentOffsetHeight, err = buildOffsetFile(tip, offsetfinished)
		if err != nil {
			panic(err)
		}
	} else {
		currentOffsetHeight = restoreCurrentOffset()
		height = restoreHeight()
		// if there is a offset file, we should pass true to offsetfinished
		// to let stopParse() know that it shouldn't delete offsetfile
		offsetfinished <- true
	}

	return currentOffsetHeight, height, nil
}

// restoreHeight restores height from simutil.HeightFilePath
func restoreHeight() int32 {

	var height int32

	// if there is a heightfile, get the height from that
	// heightFile saves the last block that was written to ttldb
	if simutil.HasAccess(simutil.HeightFilePath) {
		heightFile, err := os.OpenFile(
			simutil.HeightFilePath, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			panic(err)
		}
		var t [4]byte
		_, err = heightFile.Read(t[:])
		if err != nil {
			panic(err)
		}
		height = simutil.BtI32(t[:])
	}
	return height
}

// restoreCurrentOffset restores the currentOffsetHeight
// from simutil.CurrentOffsetFilePath
func restoreCurrentOffset() int32 {

	var currentOffsetHeight int32

	// grab the last block height from currentoffsetheight
	// currentoffsetheight saves the last height from the offsetfile
	var currentOffsetHeightByte [4]byte

	currentOffsetHeightFile, err := os.OpenFile(
		simutil.CurrentOffsetFilePath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		panic(err)
	}
	_, err = currentOffsetHeightFile.Read(currentOffsetHeightByte[:])
	if err != nil {
		panic(err)
	}

	currentOffsetHeightFile.Read(currentOffsetHeightByte[:])
	currentOffsetHeight = simutil.BtI32(currentOffsetHeightByte[:])

	return currentOffsetHeight
}

// restoreProofFileOffset restores Poffset from simutil.LastPOffsetFilePath
func restoreLastProofFileOffset() uint32 {

	// Gives the location of where a particular block height's proofs are
	// Basically an index
	var pOffset uint32

	if simutil.HasAccess(simutil.LastPOffsetFilePath) {
		pOffsetCurrentOffsetFile, err := os.OpenFile(
			simutil.LastPOffsetFilePath,
			os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			panic(err)
		}
		pOffset, err = simutil.GetPOffsetNum(pOffsetCurrentOffsetFile)
		if err != nil {
			panic(err)
		}
		fmt.Println("Poffset restored to", pOffset)

	}
	return pOffset
}

// SaveGenproofs saves the state of genproofs so that when the
// user restarts, they'll be able to resume.
// Saves height for genproofs, misc forest data, and pOffset
func saveGenproofsData(
	forest *utreexo.Forest,
	pOffset uint32,
	height int32) error {

	/*
	** Open files
	 */
	lastPOffsetFile, err := os.OpenFile(
		simutil.LastPOffsetFilePath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		panic(err)
	}
	heightFile, err := os.OpenFile(
		simutil.HeightFilePath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		panic(err)
	}
	miscForestFile, err := os.OpenFile(
		simutil.MiscForestFilePath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		panic(err)
	}

	/*
	** Write to files
	 */
	_, err = heightFile.WriteAt(simutil.I32tB(height), 0)
	if err != nil {
		panic(err)
	}
	heightFile.Close()

	// write other misc forest data
	err = forest.WriteForest(miscForestFile)
	if err != nil {
		panic(err)
	}
	// write pOffset
	_, err = lastPOffsetFile.WriteAt(
		simutil.U32tB(pOffset), 0)
	if err != nil {
		panic(err)
	}

	return nil
}

// saveIBDsimData saves the state of ibdsim so that when the
// user restarts, they'll be able to resume.
// Saves height for ibdsim and pollard itself
func saveIBDsimData(height int32, p utreexo.Pollard) {
	pHeightFile, err := os.OpenFile(
		simutil.PollardHeightFilePath, os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}

	// write to the heightfile
	_, err = pHeightFile.WriteAt(simutil.U32tB(uint32(height)), 0)
	if err != nil {
		panic(err)
	}
	pHeightFile.Close()

	pollardFile, err := os.OpenFile(simutil.PollardFilePath,
		os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}
	err = p.WritePollard(pollardFile)
	if err != nil {
		panic(err)
	}
	pollardFile.Close()

}
