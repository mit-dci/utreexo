package bridge

import (
	"os"

	"github.com/btcsuite/btcd/wire"
	"github.com/mit-dci/utreexo/cmd/util"
	"github.com/mit-dci/utreexo/utreexo"
)

// createOffsetData restores the offsetfile needed to index the
// blocks in the raw blk*.dat and raw rev*.dat files.
func createOffsetData(
	net wire.BitcoinNet, offsetFinished chan bool) (
	lastIndexOffsetHeight int32, err error) {

	// Set the Block Header hash
	// buildOffsetFile matches the header hash to organize
	// for blk*.dat files
	tip, err := util.GenHashForNet(net)
	if err != nil {
		return 0, err
	}

	err = util.BuildRevOffsetFile()
	if err != nil {
		return 0, err
	}
	lastIndexOffsetHeight, err = buildOffsetFile(*tip)
	if err != nil {
		return 0, err
	}

	offsetFinished <- true

	return
}

// restoreLastProofFileOffset restores POffset from util.LastPOffsetFilePath
// pOffset represents the location of where a particular block height's proofs
// are. Basically an index.
func restoreLastProofFileOffset() (pOffset int32, err error) {

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
	return
}

// createForest initializes forest
func createForest() (forest *utreexo.Forest, err error) {

	// Where the forestfile exists
	forestFile, err := os.OpenFile(
		util.ForestFilePath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}

	// Restores all the forest data
	forest = utreexo.NewForest(forestFile)

	return
}

// restoreForest restores forest fields based off the existing forestdata
// on disk.
func restoreForest() (forest *utreexo.Forest, err error) {

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

	return
}

// restoreHeight restores height from util.ForestLastSyncedBlockHeightFilePath
func restoreHeight() (height int32, err error) {

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
	return
}

// restoreLastIndexOffsetHeight restores the lastIndexOffsetHeight
func restoreLastIndexOffsetHeight(offsetFinished chan bool) (
	lastIndexOffsetHeight int32, err error) {

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

	return
}
