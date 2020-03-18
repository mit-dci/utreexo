package bridge

import (
	"fmt"
	"os"

	"github.com/btcsuite/btcd/wire"
	"github.com/mit-dci/utreexo/cmd/util"
	"github.com/mit-dci/utreexo/utreexo"
)

// initBridgeNodeState attempts to load and initialize the chain state from the disk.
// If a chain state is not present, chain is initialized to the genesis
// returns forest, height, lastIndexOffsetHeight, pOffset and error
func initBridgeNodeState(net wire.BitcoinNet, offsetFinished chan bool) (
	forest *utreexo.Forest, height int32, lastIndexOffsetHeight int32,
	pOffset int32, err error) {

	var offsetInitialized, forestInitialized bool

	// bool to check if the offsetfile is present
	offsetInitialized = util.HasAccess(util.OffsetFilePath)

	// Default behavior is that the user should delete all offsetdata
	// if they have new blk*.dat files to sync
	// User needs to re-index blk*.dat files when added new files to sync
	if offsetInitialized {
		var err error
		lastIndexOffsetHeight, err = restoreLastIndexOffsetHeight(offsetFinished)
		if err != nil {
			return nil, 0, 0, 0, err
		}
	} else {
		var err error
		fmt.Println("Offsetfile not present. Indexing offset for blocks blk*.dat files...")
		lastIndexOffsetHeight, err = createOffsetData(net, offsetFinished)
		if err != nil {
			return nil, 0, 0, 0, err
		}
	}

	// bool to check if the forestdata is present
	forestInitialized = util.HasAccess(util.ForestFilePath)

	if forestInitialized {
		var err error
		fmt.Println("Has access to forestdata, resuming")
		forest, err = restoreForest()
		if err != nil {
			return nil, 0, 0, 0, err
		}
		height, err = restoreHeight()
		if err != nil {
			return nil, 0, 0, 0, err
		}
		pOffset, err = restoreLastProofFileOffset()
		if err != nil {
			return nil, 0, 0, 0, err
		}
	} else {
		var err error
		fmt.Println("Creating new forestdata")
		forest, err = createForest()
		if err != nil {
			return nil, 0, 0, 0, err
		}
	}

	return
}

// saveBridgeNodeData saves the state of the bridgenode so that when the
// user restarts, they'll be able to resume.
// Saves height, forest fields, and pOffset
func saveBridgeNodeData(
	forest *utreexo.Forest, pOffset int32, height int32) error {

	/*
	** Open files
	 */
	lastPOffsetFile, err := os.OpenFile(
		util.LastPOffsetFilePath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	heightFile, err := os.OpenFile(
		util.ForestLastSyncedBlockHeightFilePath,
		os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	miscForestFile, err := os.OpenFile(
		util.MiscForestFilePath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}

	/*
	** Write to files
	 */
	_, err = heightFile.WriteAt(util.I32tB(height), 0)
	if err != nil {
		return err
	}
	// write other misc forest data
	err = forest.WriteForest(miscForestFile)
	if err != nil {
		return err
	}
	// write pOffset
	_, err = lastPOffsetFile.WriteAt(
		util.I32tB(pOffset), 0)
	if err != nil {
		return err
	}

	return nil
}
