package bridgenode

import (
	"fmt"
	"os"
	"runtime/pprof"
	"runtime/trace"
	"sync"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/mit-dci/utreexo/util"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// build the bridge node / proofs
func BuildProofs(
	param chaincfg.Params, dataDir string,
	forestInRam, forestCached bool, sig chan bool) error {

	// Channel to alert the tell the main loop it's ok to exit
	haltRequest := make(chan bool, 1)

	// Waitgroup to alert stopBuildProofs() that revoffet and offset has
	// been finished
	offsetFinished := make(chan bool, 1)

	// Channel for stopBuildProofs() to wait
	haltAccept := make(chan bool, 1)

	// Handle user interruptions
	go stopBuildProofs(sig, offsetFinished, haltRequest, haltAccept)

	// Creates all the directories needed for bridgenode
	util.MakePaths()

	// Init forest and variables. Resumes if the data directory exists
	forest, height, knownTipHeight, err :=
		initBridgeNodeState(param, dataDir, forestInRam, forestCached, offsetFinished)
	if err != nil {
		fmt.Printf("initialization error.  If your .blk and .dat files are ")
		fmt.Printf("not in %s, specify alternate path with -datadir\n.", dataDir)
		return err
	}
	// for testing only
	// knownTipHeight = 32500

	ttlpath := "utree/" + param.Name + "ttldb"
	// Open leveldb
	o := opt.Options{CompactionTableSizeMultiplier: 8}
	lvdb, err := leveldb.OpenFile(ttlpath, &o)
	if err != nil {
		fmt.Printf("initialization error.  If your .blk and .dat files are ")
		fmt.Printf("not in %s, specify alternate path with -datadir\n.", dataDir)
		return err
	}
	defer lvdb.Close()

	// For ttl value writing
	var batchwg sync.WaitGroup
	dbWriteChan := make(chan ttlRawBlock, 10)
	//	dbReadDeleteChan := make(chan ttlFromBlock, 10)

	// Start 16 workers. Just an arbitrary number
	for j := 0; j < 16; j++ {
		go DbWorker(dbWriteChan, lvdb, &batchwg)
	}

	// To send/receive blocks from blockreader()
	blockAndRevReadQueue := make(chan BlockAndRev, 10)

	// Reads block asynchronously from .dat files
	// Reads util the lastIndexOffsetHeight
	go BlockAndRevReader(blockAndRevReadQueue, dataDir, "",
		knownTipHeight, height)
	proofChan := make(chan []byte, 10) // channel for the bytes of proof data
	//	ttlChan := make(chan util.TtlBlock, 10) // channel for ttls to be put in old proofs
	var fileWait sync.WaitGroup
	go flatFileWriter(proofChan, &fileWait)

	fmt.Println("Building Proofs and ttldb...")

	var stop bool // bool for stopping the main loop

	for ; height != knownTipHeight && !stop; height++ {

		// Receive txs from the asynchronous blk*.dat reader
		bnr := <-blockAndRevReadQueue

		// Writes the new txos to leveldb, and generates TTL for txos spent in the block
		ParseBlockForDB(bnr, dbWriteChan, &batchwg)

		// Get the add and remove data needed from the block & undo block
		blockAdds, delLeaves, err := blockToAddDel(bnr)
		if err != nil {
			return err
		}

		// use the accumulator to get inclusion proofs, and produce a block
		// proof with all data needed to verify the block
		// this also includes TTL values, but they are unpopulated right now, because
		// we don't yet know when the UTXOs in this block die.
		ud, err := genUData(delLeaves, forest, bnr.Height)
		if err != nil {
			return err
		}

		// convert UData struct to bytes
		b := ud.ToBytes()

		// Add to WaitGroup and send data to channel to be written
		// to disk
		fileWait.Add(1)
		proofChan <- b

		ud.AccProof.SortTargets()

		// fmt.Printf("h %d adds %d targets %d\n",
		// 	height, len(blockAdds), len(ud.AccProof.Targets))

		// TODO: Don't ignore undoblock
		// Modifies the forest with the given TXINs and TXOUTs
		_, err = forest.Modify(blockAdds, ud.AccProof.Targets)
		if err != nil {
			return err
		}

		if bnr.Height%100 == 0 {
			fmt.Println("On block :", bnr.Height+1)
		}

		// Check if stopSig is no longer false
		// stop = true makes the loop exit
		select {
		case stop = <-haltRequest: // receives true from stopBuildProofs()
		default:
		}
	}

	// wait until dbWorker() has written to the ttldb file
	// allows leveldb to close gracefully
	batchwg.Wait()

	// Wait for the file workers to finish
	fileWait.Wait()

	// Save the current state so genproofs can be resumed
	err = saveBridgeNodeData(forest, height, forestInRam)
	if err != nil {
		panic(err)
	}

	fmt.Println("Done writing")

	// Tell stopBuildProofs that it's ok to exit
	haltAccept <- true
	return nil

}

// stopBuildProofs listens for the signal from the OS and initiates an exit sequence
func stopBuildProofs(
	sig, offsetfinished, haltRequest, haltAccept chan bool) {

	// Listen for SIGINT, SIGQUIT, SIGTERM
	// Also listen for an unrequested haltAccept which means upstream is finshed
	// and to end this goroutine
	select {
	case <-haltAccept:
		return
	case <-sig:
	}

	trace.Stop()
	pprof.StopCPUProfile()

	// Sometimes there are bugs that make the program run forever.
	// Utreexo binary should never take more than 10 seconds to exit
	go func() {
		time.Sleep(60 * time.Second)
		fmt.Println("Program timed out. Force quitting. Data likely corrupted")
		os.Exit(1)
	}()

	// Tell the user that the sig is received
	fmt.Println("User exit signal received. Exiting...")

	select {
	// If offsetfile is there or was built, don't remove it
	case <-offsetfinished:
		haltRequest <- true
	// If nothing is received, delete offsetfile and other directories
	// Don't wait for done channel from the main BuildProofs() for loop
	default:
		fmt.Println("offsetfile incomplete, removing...")
		// May not work sometimes.
		err := os.RemoveAll(util.OffsetDirPath)
		if err != nil {
			fmt.Println("ERR. offsetdata/ directory not removed. Please manually remove it.")
		}
		fmt.Println("Exiting...")
		os.Exit(0)
	}

	// Wait until BuildProofs() or buildOffsetFile() says it's ok to exit
	<-haltAccept
	os.Exit(0)
}
