package bridge

import (
	"os"

	"github.com/mit-dci/utreexo/cmd/util"
	"github.com/mit-dci/utreexo/utreexo"
)

// createOffsetData restores the offsetfile needed to index the
// blocks in the raw blk*.dat files.
func createOffsetData(
	isTestnet bool, offsetFinished chan bool) (int32, error) {

	var lastIndexOffsetHeight int32

	// Set the Block Header hash
	// buildOffsetFile matches the header hash to organize
	var tip util.Hash
	if isTestnet == true {
		tip = util.TestNet3GenHash
	} else {
		tip = util.MainnetGenHash
	}

	var err error
	lastIndexOffsetHeight, err = buildOffsetFile(tip, offsetFinished)
	if err != nil {
		return 0, err
	}

	return lastIndexOffsetHeight, nil
}

// restoreLastProofFileOffset restores POffset from util.LastPOffsetFilePath
func restoreLastProofFileOffset() (int32, error) {

	// Gives the location of where a particular block height's proofs are
	// Basically an index
	var pOffset int32

	if util.HasAccess(util.LastPOffsetFilePath) {
		f, err := os.OpenFile(
			util.LastPOffsetFilePath,
			os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return 0, err
		}
		var pOffsetByte [4]byte
		_, err = f.ReadAt(pOffsetByte[:], 0)
		if err != nil {
			return 0, err
		}
		pOffset = util.BtI32(pOffsetByte[:])

	}
	return pOffset, nil
}

// createForest initializes forest
func createForest() (*utreexo.Forest, error) {

	// Where the forestfile exists
	forestFile, err := os.OpenFile(
		util.ForestFilePath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}

	// Restores all the forest data
	forest := utreexo.NewForest(forestFile)

	return forest, nil
}

// restoreForest restores forest fields based off the existing forestdata
// on disk.
func restoreForest() (*utreexo.Forest, error) {

	var forest *utreexo.Forest

	// Where the forestfile exists
	forestFile, err := os.OpenFile(
		util.ForestFilePath, os.O_RDWR, 0400)
	if err != nil {
		return nil, err
	}
	// Where the misc forest data exists
	miscForestFile, err := os.OpenFile(
		util.MiscForestFilePath, os.O_RDONLY, 0400)
	if err != nil {
		return nil, err
	}

	forest, err = utreexo.RestoreForest(miscForestFile, forestFile)
	if err != nil {
		return nil, err
	}

	return forest, nil
}

// restoreHeight restores height from util.ForestLastSyncedBlockHeightFilePath
func restoreHeight() (int32, error) {

	var height int32

	// if there is a heightfile, get the height from that
	// heightFile saves the last block that was written to ttldb
	if util.HasAccess(util.ForestLastSyncedBlockHeightFilePath) {
		heightFile, err := os.OpenFile(
			util.ForestLastSyncedBlockHeightFilePath,
			os.O_RDONLY, 0400)
		if err != nil {
			return 0, err
		}
		var t [4]byte
		_, err = heightFile.Read(t[:])
		if err != nil {
			return 0, err
		}
		height = util.BtI32(t[:])
	}
	return height, nil
}

// restoreLastIndexOffsetHeight restores the lastIndexOffsetHeight
func restoreLastIndexOffsetHeight(offsetFinished chan bool) (int32, error) {

	var lastIndexOffsetHeight int32

	// grab the last block height from currentoffsetheight
	// currentoffsetheight saves the last height from the offsetfile
	var lastIndexOffsetHeightByte [4]byte

	f, err := os.OpenFile(
		util.LastIndexOffsetHeightFilePath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return 0, err
	}
	_, err = f.Read(lastIndexOffsetHeightByte[:])
	if err != nil {
		return 0, err
	}

	f.Read(lastIndexOffsetHeightByte[:])
	lastIndexOffsetHeight = util.BtI32(lastIndexOffsetHeightByte[:])

	// if there is a offset file, we should pass true to offsetFinished
	// to let stopParse() know that it shouldn't delete offsetfile
	offsetFinished <- true

	return lastIndexOffsetHeight, nil
}
