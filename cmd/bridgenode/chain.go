package bridge

import (
	"fmt"
	"os"
	"sync"

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

	// Default behavior is that the user should delete all offsetdata
	// if they have new blk*.dat files to sync
	// User needs to re-index blk*.dat files when added new files to sync

	// Both the blk*.dat offset and rev*.dat offset is checked at the same time
	// If either is incomplete or not complete, they're both removed and made
	// anew
	// Check if the offsetfiles for both rev*.dat and blk*.dat are present
	if util.HasAccess(util.OffsetFilePath) && util.HasAccess(
		util.RevOffsetFilePath) {
		var err error
		lastIndexOffsetHeight, err = restoreLastIndexOffsetHeight(
			offsetFinished)
		if err != nil {
			return nil, 0, 0, 0, err
		}
	} else {
		var err error
		fmt.Println("Offsetfile not present or half present." +
			"Indexing offset for blocks blk*.dat files...")
		lastIndexOffsetHeight, err = createOffsetData(net, offsetFinished)
		if err != nil {
			return nil, 0, 0, 0, err
		}
	}

	// Check if the forestdata is present
	if util.HasAccess(util.ForestFilePath) {
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

	var fileWait sync.WaitGroup
	fileWait.Add(1)
	go func() error {
		heightFile, err := os.OpenFile(
			util.ForestLastSyncedBlockHeightFilePath,
			os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return err
		}
		_, err = heightFile.WriteAt(util.I32tB(height), 0)
		if err != nil {
			return err
		}
		fileWait.Done()
		return nil
	}()
	fileWait.Add(1)
	// write other misc forest data
	go func() error {
		miscForestFile, err := os.OpenFile(
			util.MiscForestFilePath, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return err
		}
		err = forest.WriteForest(miscForestFile)
		if err != nil {
			return err
		}
		fileWait.Done()
		return nil
	}()
	fileWait.Add(1)
	// write pOffset
	go func() error {
		lastPOffsetFile, err := os.OpenFile(
			util.LastPOffsetFilePath, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return err
		}
		_, err = lastPOffsetFile.WriteAt(
			util.I32tB(pOffset), 0)
		if err != nil {
			return err
		}
		fileWait.Done()
		return nil
	}()

	fileWait.Wait()
	return nil
}
