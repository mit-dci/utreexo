package bridgenode

import (
	"fmt"
	"os"
	"runtime/pprof"
	"runtime/trace"
	"sync"
	"time"

	"github.com/mit-dci/utreexo/btcacc"

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
func BuildProofs(cfg *Config, sig chan bool) error {
	// Channel to alert the tell the main loop it's ok to exit
	haltRequest := make(chan bool, 1)

	// Waitgroup to alert stopBuildProofs() that revoffet and offset has
	// been finished
	offsetFinished := make(chan bool, 1)

	// Channel for stopBuildProofs() to wait
	haltAccept := make(chan bool, 1)

	// Handle user interruptions
	go stopBuildProofs(cfg, sig, offsetFinished, haltRequest, haltAccept)

	// Init forest and variables. Resumes if the data directory exists
	forest, height, knownTipHeight, err :=
		InitBridgeNodeState(cfg, offsetFinished)
	if err != nil {
		err := fmt.Errorf("initialization error.  If your .blk and .dat files are "+
			"not in %s, specify alternate path with -datadir\n.", cfg.BlockDir)
		return err
	}

	// Open leveldb
	o := opt.Options{
		CompactionTableSizeMultiplier: 8,
		Compression:                   opt.NoCompression,
	}

	// init ttldb
	lvdb, err := leveldb.OpenFile(cfg.UtreeDir.Ttldb, &o)
	if err != nil {
		err := fmt.Errorf("initialization error.  If your .blk and .dat files are "+
			"not in %s, specify alternate path with -datadir\n.", cfg.BlockDir)
		return err
	}
	defer lvdb.Close()

	var dbwg sync.WaitGroup

	// To send/receive blocks from blockreader()
	blockAndRevReadQueue := make(chan BlockAndRev, 10) // blocks from disk to processing

	dbWriteChan := make(chan ttlRawBlock, 10)      // from block processing to db worker
	ttlResultChan := make(chan ttlResultBlock, 10) // from db worker to flat ttl writer
	proofChan := make(chan btcacc.UData, 10)       // from proof processing to proof writer

	// Start 16 workers. Just an arbitrary number
	// I think we can only have one dbworker now, since it needs to all happen in order?
	go DbWorker(dbWriteChan, ttlResultChan, lvdb, &dbwg)

	// Reads block asynchronously from .dat files
	// Reads util the lastIndexOffsetHeight
	go BlockAndRevReader(blockAndRevReadQueue, cfg, knownTipHeight, height)

	var fileWait sync.WaitGroup

	go flatFileWorker(proofChan, ttlResultChan, cfg.UtreeDir, &fileWait)

	fmt.Println("Building Proofs and ttldb...")

	var stop bool // bool for stopping the main loop

	for ; height != knownTipHeight && !stop; height++ {
		if cfg.quitAt != -1 && int(height) == cfg.quitAt {
			fmt.Println("quitAfter value reached. Quitting...")

			// TODO this is ugly, have an actually quitter. Need to refactor
			// a bunch of things..
			trace.Stop()
			pprof.StopCPUProfile()
			break
		}
		// Receive txs from the asynchronous blk*.dat reader
		bnr := <-blockAndRevReadQueue

		// start waitgroups, beyond this point we have to finish all the
		// disk writes for this iteration of the loop
		dbwg.Add(1)     // DbWorker calls Done()
		fileWait.Add(2) // flatFileWorker calls Done() when done writing ttls and proof.

		// Writes the new txos to leveldb,
		// and generates TTL for txos spent in the block
		// also wants the skiplist to omit 0-ttl txos
		dbWriteChan <- ParseBlockForDB(bnr)

		// Get the add and remove data needed from the block & undo block
		// wants the skiplist to omit proofs
		blockAdds, delLeaves, err := blockToAddDel(bnr)
		if err != nil {
			return err
		}

		// use the accumulator to get inclusion proofs, and produce a block
		// proof with all data needed to verify the block
		ud, err := btcacc.GenUData(delLeaves, forest, bnr.Height)
		if err != nil {
			return err
		}
		// We don't know the TTL values, but know how many spots to allocate
		ud.TxoTTLs = make([]int32, len(blockAdds))
		// send proof udata to channel to be written to disk
		proofChan <- ud

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
	dbwg.Wait()

	fmt.Printf("blocked on fileWait\n")
	// Wait for the file workers to finish
	fileWait.Wait()

	// Save the current state so genproofs can be resumed
	err = saveBridgeNodeData(forest, height, cfg)
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
	cfg *Config, sig, offsetfinished, haltRequest, haltAccept chan bool) {

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
		time.Sleep(1000 * time.Second)
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
		err := os.RemoveAll(cfg.UtreeDir.OffsetDir.base)
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
