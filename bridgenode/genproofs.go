package bridgenode

import (
	"bytes"
	"fmt"
	"os"
	"runtime/pprof"
	"runtime/trace"
	"sync"
	"time"

	"github.com/mit-dci/utreexo/accumulator"
	"github.com/mit-dci/utreexo/btcacc"
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

/* Problem : BlockAndRevReader keeps going after the stop signal happens.
So it fills up buffers with ~10 blocks, and will keep going forever;
need to tell BlockAndRevReader to stop, and then let everything else
(including the main loop with Modify() I guess) keep running until that
buffer clears out.

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
	forest, finishedHeight, err := InitBridgeNodeState(cfg, offsetFinished)
	if err != nil {
		err := fmt.Errorf("initialization error: %s.  If your .blk and .dat "+
			"files are not in %s, specify alternate path with -datadir\n.",
			err.Error(), cfg.BlockDir)
		return err
	}

	fmt.Printf("Starting forest: %s\n", forest.ToString())

	// BlockAndRevReader will push blocks into here
	blockAndRevProofChan := make(chan blockAndRev, 10) // blocks for accumulator
	blockAndRevTTLChan := make(chan blockAndRev, 10)   // same thing, but for TTL
	ttlResultChan := make(chan ttlResultBlock, 10)     // from lookup to flat ttl writer
	proofChan := make(chan btcacc.UData, 10)           // to flat writer
	undoChan := make(chan accumulator.UndoBlock, 10)   // to undoblock writer
	numLeavesChan := make(chan int, 10)                // empty leaves for TTLs

	fileWait := new(sync.WaitGroup)

	// Reads block asynchronously from .dat files
	// Reads util the lastIndexOffsetHeight

	go BlockAndRevReader(
		blockAndRevProofChan, blockAndRevTTLChan,
		haltRequest, fileWait, cfg, finishedHeight)

	go flatFileWorkerProof(proofChan, cfg.UtreeDir, fileWait)
	go flatFileWorkerUndo(undoChan, cfg.UtreeDir, fileWait)
	go flatFileWorkerTTL(ttlResultChan, numLeavesChan, cfg.UtreeDir, fileWait)

	go BNRTTLSpliter(blockAndRevTTLChan, ttlResultChan, cfg.UtreeDir)

	fmt.Println("Building Proofs and ttls...")

	for {
		// fmt.Printf("block on blockAndRevProofChan read?\n")
		// Receive txs from the asynchronous blk*.dat reader
		bnr, open := <-blockAndRevProofChan
		if !open { // channel is closed by BlockAndRevReader & empty, we're done
			break
		}

		if bnr.Blk == nil {
			fmt.Printf("h %d empty block ", bnr.Height)
			panic("empty")
		}
		// Get the add and remove data needed from the block & undo block
		// wants the skiplist to omit proofs
		blockAdds, delLeaves, err := bnr.toAddDel()
		if err != nil {
			return err
		}

		numLeavesChan <- len(blockAdds)

		// use the accumulator to get inclusion proofs, and produce a block
		// proof with all data needed to verify the block
		ud, err := btcacc.GenUData(delLeaves, forest, bnr.Height)
		if err != nil {
			return err
		}
		// We don't know the TTL values, but know how many spots to allocate
		ud.TxoTTLs = make([]int32, bnr.outCount)

		// fmt.Printf("block on proofchan?\n")
		// send proof udata to channel to be written to disk
		proofChan <- ud

		undoblock, err := forest.Modify(blockAdds, ud.AccProof.Targets)
		if err != nil {
			return err
		}
		undoblock.Height = bnr.Height // set undoBlocks Height
		// send undoBlock data to undo channel to be written to the disk
		// fmt.Printf("block on undochan?\n")
		undoChan <- *undoblock

		finishedHeight = bnr.Height
		if finishedHeight%1000 == 0 {
			fmt.Printf("Finished block %d of max %d\n",
				finishedHeight, cfg.quitAfter)
		}

	}

	// Wait for the file workers to finish
	fileWait.Wait()

	// Save the current state so genproofs can be resumed
	err = saveBridgeNodeData(forest, finishedHeight, cfg)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Done writing. Height %d Forest: %s",
		finishedHeight, forest.ToString())

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

// go through all the proofs and just try to deserialize them
func VerifyProofs(cfg *Config) error {

	for h := int32(1); h < cfg.quitAfter; h++ {
		if h%100 == 0 {
			fmt.Printf("verify h %d\n", h)
		}
		udb, err := GetUDataBytesFromFile(cfg.UtreeDir.ProofDir, h)
		if err != nil {
			return fmt.Errorf("GetUDataBytesFromFile %s\n", err.Error())
		}
		// fmt.Printf("got udb %d bytes:\n%x\n", len(udb), udb)
		buf := bytes.NewBuffer(udb)
		// deserialize to find errors
		var ud btcacc.UData
		err = ud.Deserialize(buf)
		if err != nil {
			fmt.Printf("serveBlocksWorker h %d deser error %s\n", h, err.Error())
			fmt.Printf("ttls: %v targets %s\n", ud.TxoTTLs, ud.AccProof.ToString())
			fmt.Printf("udb: %x\n", udb)
			return err
		}
		// if len(ud.AccProof.Targets) != 0 {
		// fmt.Printf("h %d has %d targets\n", h, len(ud.AccProof.Targets))
		// }
	}
	return nil
}
