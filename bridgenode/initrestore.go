package bridgenode

import (
	"encoding/binary"
	"fmt"
	"os"

	"github.com/mit-dci/utreexo/accumulator"
	"github.com/mit-dci/utreexo/util"
)

// initBridgeNodeState attempts to load and initialize the chain state from the disk.
// If a chain state is not present, chain is initialized to the genesis
// returns forest, height, lastIndexOffsetHeight, pOffset and error
func InitBridgeNodeState(cfg *Config, offsetFinished chan bool) (forest *accumulator.Forest,
	height int32, knownTipHeight int32, err error) {

	// Default behavior is that the user should delete all offsetdata
	// if they have new blk*.dat files to sync
	// User needs to re-index blk*.dat files when added new files to sync

	// Both the blk*.dat offset and rev*.dat offset is checked at the same time
	// If either is incomplete or not complete, they're both removed and made
	// anew
	// Check if the offsetfiles for both rev*.dat and blk*.dat are present
	if util.HasAccess(cfg.UtreeDir.OffsetDir.OffsetFile) {
		knownTipHeight, err = restoreLastIndexOffsetHeight(cfg.UtreeDir.OffsetDir, offsetFinished)
		if err != nil {
			err = fmt.Errorf("restoreLastIndexOffsetHeight error: %s", err.Error())
			return
		}
	} else {
		fmt.Println("Offsetfile not present or half present. " +
			"Indexing offset for blocks blk*.dat files...")
		knownTipHeight, err = createOffsetData(cfg, offsetFinished)
		if err != nil {
			err = fmt.Errorf("createOffsetData error: %s", err.Error())
			return
		}
		fmt.Printf("tip height %d\n", knownTipHeight)
	}

	if checkForestExists(cfg) {
		fmt.Println("Has access to forest, resuming")
		forest, err = restoreForest(cfg)
		if err != nil {
			err = fmt.Errorf("restoreForest error: %s", err.Error())
			return
		}
		height, err = restoreHeight(cfg)
		if err != nil {
			err = fmt.Errorf("restoreHeight error: %s", err.Error())
			return
		}
	} else {
		fmt.Println("Creating new forest")
		// TODO Add a path for CowForest here
		forest, err = createForest(cfg)
		height = 1 // note that blocks start at 1, block 0 doesn't go into set
		if err != nil {
			err = fmt.Errorf("createForest error: %s", err.Error())
			return
		}
	}

	return
}

// saveBridgeNodeData saves the state of the bridgenode so that when the
// user restarts, they'll be able to resume.
// Saves height, forest fields, and pOffset
func saveBridgeNodeData(
	forest *accumulator.Forest, height int32, cfg *Config) error {

	switch cfg.forestType {
	case ramForest:
		forestFile, err := os.OpenFile(
			cfg.UtreeDir.ForestDir.forestFile,
			os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return err
		}
		err = forest.WriteForestToDisk(forestFile, true, false)
		if err != nil {
			return err
		}

	case cowForest:
		err := forest.WriteForestToDisk(nil, false, true)
		if err != nil {
			return err
		}
	}

	heightFile, err := os.OpenFile(
		cfg.UtreeDir.ForestDir.forestLastSyncedBlockHeightFile,
		os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	err = binary.Write(heightFile, binary.BigEndian, height)
	if err != nil {
		return err
	}

	// write other misc forest data
	miscForestFile, err := os.OpenFile(
		cfg.UtreeDir.ForestDir.miscForestFile, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	err = forest.WriteMiscData(miscForestFile)
	if err != nil {
		return err
	}

	return nil
}

// createOffsetData restores the offsetfile needed to index the
// blocks in the raw blk*.dat and raw rev*.dat files.
func createOffsetData(
	cfg *Config, offsetFinished chan bool) (
	lastIndexOffsetHeight int32, err error) {

	// Set the Block Header hash
	// buildOffsetFile matches the header hash to organize
	// for blk*.dat files
	hash, err := util.GenHashForNet(cfg.params)
	if err != nil {
		return 0, err
	}

	// TODO allow the user to pass a custom offsetfile path and
	// custom lastOffsetHeight path instead of just ""
	lastIndexOffsetHeight, err = buildOffsetFile(cfg, *hash, "", "")
	if err != nil {
		return 0, err
	}

	offsetFinished <- true

	return
}

// createForest initializes forest
func createForest(cfg *Config) (
	forest *accumulator.Forest, err error) {

	switch cfg.forestType {
	case ramForest:
		forest = accumulator.NewForest(nil, false, "", 0)
		return
	case cowForest:
		forest = accumulator.NewForest(nil, false, cfg.UtreeDir.ForestDir.cowForestDir, cfg.cowMaxCache)
		return
	default:
		var cache bool
		if cfg.forestType == cacheForest {
			cache = true
		}

		// Where the forestfile exists
		forestFile, err := os.OpenFile(
			cfg.UtreeDir.ForestDir.forestFile, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return nil, err
		}

		// Restores all the forest data
		forest = accumulator.NewForest(forestFile, cache, "", 0)
	}

	return
}

// restoreForest restores forest fields based off the existing forestdata
// on disk.
func restoreForest(cfg *Config) (
	forest *accumulator.Forest, err error) {

	switch cfg.forestType {
	case cowForest:
		var miscForestFile *os.File
		// Where the misc forest data exists
		miscForestFile, err = os.OpenFile(cfg.UtreeDir.ForestDir.miscForestFile, os.O_RDONLY, 0400)
		if err != nil {
			return nil, err
		}
		forest, err = accumulator.RestoreForest(
			miscForestFile, nil, false, false, cfg.UtreeDir.ForestDir.cowForestDir, cfg.cowMaxCache)

	default:
		var (
			inRam bool
			cache bool
		)
		switch cfg.forestType {
		case ramForest:
			inRam = true
		case cacheForest:
			cache = true
		}

		var forestFile *os.File
		var miscForestFile *os.File
		// Where the forestfile exists
		forestFile, err = os.OpenFile(cfg.UtreeDir.ForestDir.forestFile, os.O_RDWR, 0400)
		if err != nil {
			return
		}
		// Where the misc forest data exists
		miscForestFile, err = os.OpenFile(cfg.UtreeDir.ForestDir.miscForestFile, os.O_RDONLY, 0400)
		if err != nil {
			return
		}

		forest, err = accumulator.RestoreForest(
			miscForestFile, forestFile, inRam, cache, "", 0)

	}

	return
}

// restoreHeight restores height from util.ForestLastSyncedBlockHeightFileName
func restoreHeight(cfg *Config) (height int32, err error) {
	// if there is a heightfile, get the height from that
	// heightFile saves the last block that was written to ttldb
	if util.HasAccess(cfg.UtreeDir.ForestDir.forestLastSyncedBlockHeightFile) {
		heightFile, err := os.OpenFile(
			cfg.UtreeDir.ForestDir.forestLastSyncedBlockHeightFile,
			os.O_RDONLY, 0400)
		if err != nil {
			return 0, err
		}
		err = binary.Read(heightFile, binary.BigEndian, &height)
		if err != nil {
			return 0, err
		}
	} else {
		return 0, fmt.Errorf(
			"can't read height at %s (must build before serving)\n",
			cfg.UtreeDir.ForestDir.forestLastSyncedBlockHeightFile)
	}
	return
}

// restoreLastIndexOffsetHeight restores the lastIndexOffsetHeight
func restoreLastIndexOffsetHeight(offsetDir offsetDir, offsetFinished chan bool) (
	lastIndexOffsetHeight int32, err error) {

	f, err := os.OpenFile(
		offsetDir.lastIndexOffsetHeightFile, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return 0, err
	}

	// grab the last block height from currentoffsetheight
	// currentoffsetheight saves the last height from the offsetfile
	err = binary.Read(f, binary.BigEndian, &lastIndexOffsetHeight)
	if err != nil {
		return 0, err
	}
	// if there is a offset file, we should pass true to offsetFinished
	// to let stopParse() know that it shouldn't delete offsetfile
	offsetFinished <- true

	return
}

// Check that the data for this forest type specified in the config
// is present and should be resumed off of
func checkForestExists(cfg *Config) bool {
	switch cfg.forestType {
	case cowForest:
		return util.HasAccess(cfg.UtreeDir.ForestDir.cowForestCurFile)
	default:
		return util.HasAccess(cfg.UtreeDir.ForestDir.forestFile)

	}
}
