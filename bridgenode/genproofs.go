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

/*
The pipeline:

The tricky part is that we read blocks, but that block data goes to 2 places.
It goes to the accumulator to generate a proof, and it goes to the DB to
get the TTL data.  The proof then gets written to a flat file,

BlockAndRevReader reads from bitcoind flat files
-> blockAndRevReadQueue -> read by main loop, sent to 2 places


  \--> genUData --------> forest.Modify()
   \               \-----> proofchan -> flatFileBlockWorker -> offsets
    \
     \-> ParseBlockForDB -> dbWriteChan -> dbWorker -> ttlResultChan

then flatFileTTLWorker needs both the offsets from the flatFileBlockWorker
and the TTL data from the dbWorker.


this all stays in order, so the flatFileTTLWorker grabs from the offset channel,
and then only if the offset channel is empty will it grab from ttlResultChan.
This ensures that it's always got the offset data first.
if restoring, initially it gets a flood of offset data, so this works OK because
it grabs all that before trying to look for TTL data.

*/

// build the bridge node / proofs
func BuildProofs(
	param chaincfg.Params, dataDir string,
	forestInRam, forestCached, cowForest bool, maxCachedCount int, sig chan bool) error {

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
		initBridgeNodeState(
			param,
			dataDir,
			forestInRam,
			forestCached,
			cowForest,
			maxCachedCount,
			offsetFinished,
		)

	if err != nil {
		fmt.Printf("initialization error.  If your .blk and .dat files are ")
		fmt.Printf("not in %s, specify alternate path with -datadir\n.", dataDir)
		return err
	}

	ttlpath := "utree/" + param.Name + "ttldb"

	// Open leveldb
	o := opt.Options{
		CompactionTableSizeMultiplier: 8,
		Compression:                   opt.NoCompression,
	}
	lvdb, err := leveldb.OpenFile(ttlpath, &o)
	if err != nil {
		fmt.Printf("initialization error.  If your .blk and .dat files are ")
		fmt.Printf("not in %s, specify alternate path with -datadir\n.", dataDir)
		return err
	}
	defer lvdb.Close()

	var dbwg sync.WaitGroup

	// To send/receive blocks from blockreader()
	blockAndRevReadQueue := make(chan BlockAndRev, 10) // blocks from disk to processing

	dbWriteChan := make(chan ttlRawBlock, 10)      // from block processing to db worker
	ttlResultChan := make(chan ttlResultBlock, 10) // from db worker to flat ttl writer
	proofChan := make(chan util.UData, 10)         // from proof processing to proof writer
	// Start 16 workers. Just an arbitrary number
	//	for j := 0; j < 16; j++ {
	// I think we can only have one dbworker now, since it needs to all happen in order?
	go DbWorker(dbWriteChan, ttlResultChan, lvdb, &dbwg)
	//	}

	// Reads block asynchronously from .dat files
	// Reads util the lastIndexOffsetHeight
	go BlockAndRevReader(blockAndRevReadQueue, dataDir, "",
		knownTipHeight, height)

	var fileWait sync.WaitGroup

	go flatFileWorker(proofChan, ttlResultChan, &fileWait)

	fmt.Println("Building Proofs and ttldb...")

	var stop bool // bool for stopping the main loop

	for ; height != knownTipHeight && !stop; height++ {
		// Receive txs from the asynchronous blk*.dat reader
		bnr := <-blockAndRevReadQueue

		inskip, outskip := util.DedupeBlock(&bnr.Blk)

		// start waitgroups, beyond this point we have to finish all the
		// disk writes for this iteration of the loop
		dbwg.Add(1)     // DbWorker calls Done()
		fileWait.Add(1) // flatFileWorker calls Done() (when done writing ttls)

		// Writes the new txos to leveldb,
		// and generates TTL for txos spent in the block
		// also wants the skiplist to omit 0-ttl txos
		dbWriteChan <- ParseBlockForDB(bnr, inskip, outskip)

		// Get the add and remove data needed from the block & undo block
		// wants the skiplist to omit proofs
		blockAdds, delLeaves, err := blockToAddDel(bnr, inskip, outskip)
		if err != nil {
			return err
		}

		// use the accumulator to get inclusion proofs, and produce a block
		// proof with all data needed to verify the block
		ud, err := genUData(delLeaves, forest, bnr.Height)
		if err != nil {
			return err
		}
		// We don't know the TTL values, but know how many spots to allocate
		ud.TxoTTLs = make([]int32, len(blockAdds))
		// send proof udata to channel to be written to disk
		proofChan <- ud

		// ud.AccProof.SortTargets()
		// fmt.Printf("h %d nl %d adds %d targets %d %v\n",
		// height, nl, len(blockAdds), len(ud.AccProof.Targets), ud.AccProof.Targets)

		// TODO: Don't ignore undoblock
		// Modifies the forest with the given TXINs and TXOUTs
		_, err = forest.Modify(blockAdds, ud.AccProof.Targets)
		if err != nil {
			return err
		}
		// fmt.Printf(ud.AccProof.ToString())

		// if height == 400 {
		// stop = true
		// }

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
	dbwg.Wait()

	fmt.Printf("blocked on fileWait\n")
	// Wait for the file workers to finish
	fileWait.Wait()

	// Save the current state so genproofs can be resumed
	err = saveBridgeNodeData(forest, height, forestInRam, cowForest)
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
