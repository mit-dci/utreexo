package bridgenode

import (
	"encoding/binary"
	"os"

	"github.com/mit-dci/utreexo/accumulator"
	"github.com/mit-dci/utreexo/util"
)

// createForest initializes forest
func createForest(inRam, cached bool) (forest *accumulator.Forest, err error) {
	if inRam {
		forest = accumulator.NewForest(nil, false)
		return
	}

	// Where the forestfile exists
	forestFile, err := os.OpenFile(
		util.ForestFilePath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return
	}

	// Restores all the forest data
	forest = accumulator.NewForest(forestFile, cached)

	return
}

// restoreForest restores forest fields based off the existing forestdata
// on disk.
func restoreForest(
	forestFilename, miscFilename string,
	inRam, cached bool) (forest *accumulator.Forest, err error) {

	// Where the forestfile exists
	forestFile, err := os.OpenFile(forestFilename, os.O_RDWR, 0400)
	if err != nil {
		return
	}
	// Where the misc forest data exists
	miscForestFile, err := os.OpenFile(miscFilename, os.O_RDONLY, 0400)
	if err != nil {
		return
	}

	forest, err = accumulator.RestoreForest(miscForestFile, forestFile, inRam, cached)
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
		err = binary.Read(heightFile, binary.BigEndian, &height)
		if err != nil {
			return 0, err
		}
	}
	return
}
