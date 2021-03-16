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

DATA INFLOW:
Block & Rev data comes in from BlockAndRevReader, which duplicates the block
data and sends it to both the proof path and the TTL path.

PROOF PATH:
The proof path is in the main for loop right now and not in its own worker
thread.  This path converts the block & rev data into accumulator adds & dels
with blockToAddDel(), then calls GenUData() to generate a proof for the
deletions, which it sends via proofChan to the FlatFileWriter() which writes
the proof to disk.  Then it calls Modify() on the accumulator, removing the
deleted hashes and adding new ones.

TTL PATH:
The block & rev data is first sent to BNRTTLSpliter(), which spawns 2 new
threads: TxidSortWriterWorker() and TTLLookupWorker().  BNRTTLSpliter() splits
the block up into inputs and outputs; inputs go to the TTLLookupWorker() and
outputs go to the TxidSortWriterWorker() (barely even outputs; just TXIDs and
number of outputs)

TxidSortWriterWorker() builds a flat file of per-block sorted, truncated TXIDs.
TTLLookupWorker() looks up inputs in this sorted TXID file, and obtains position
data for the TTL value of a UTXO.  We already have the TTL data for the UTXO
from the current block height and the rev data which tells the utxo creation
height.  We want to write the TTL into the TTL area of the proof block, but
we need to look up where in the block this UTXO was created, and that's what
TTLLookupWorker() gets us.  Once we have the full TTL result, we send that
via ttlResultChan to FlatFileWriter()

FLAT FILE:
FlatFileWriter() takes in proof data as well as TTL data.  When it gets a
proof block it appends that to the end of the proof file, allocating a bunch
of empty space in the beginning for TTL values.
When it gets a TTL result block, it writes the TTL values to various previous
blocks, overwriting the allocated empty (zero) data in the TTL region of the
proof block.

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
		err := fmt.Errorf("initialization error: %s.  If your .blk and .dat "+
			"files are not in %s, specify alternate path with -datadir\n.",
			err.Error(), cfg.BlockDir)
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
		err := fmt.Errorf("initialization error: %s.  If your .blk and .dat "+
			"files are not in %s, specify alternate path with -datadir\n.",
			err.Error(), cfg.BlockDir)
		return err
	}
	defer lvdb.Close()

	// BlockAndRevReader will push blocks into here
	blockAndRevProofChan := make(chan blockAndRev, 10) // blocks for accumulator
	blockAndRevTTLChan := make(chan blockAndRev, 10)   // same thing, but for TTL
	ttlResultChan := make(chan ttlResultBlock, 10)     // from db worker to flat ttl writer
	proofChan := make(chan btcacc.UData, 10)           // from proof processing to proof writer

	fileWait := new(sync.WaitGroup)

	// Reads block asynchronously from .dat files
	// Reads util the lastIndexOffsetHeight
	go BlockAndRevReader(
		blockAndRevProofChan, blockAndRevTTLChan, fileWait, cfg, knownTipHeight, height)

	go FlatFileWriter(proofChan, ttlResultChan, cfg.UtreeDir, fileWait)
	go BNRTTLSpliter(blockAndRevTTLChan, ttlResultChan)

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
		bnr := <-blockAndRevProofChan

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
			fmt.Printf("On block %d of max %d ", bnr.Height+1, knownTipHeight)
		}

		// Check if stopSig is no longer false
		// stop = true makes the loop exit
		select {
		case stop = <-haltRequest: // receives true from stopBuildProofs()
		default:
		}
	}

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
