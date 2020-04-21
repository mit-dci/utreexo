package bridgenode

import (
	"encoding/binary"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/btcsuite/btcd/wire"
	"github.com/mit-dci/utreexo/accumulator"
	"github.com/mit-dci/utreexo/util"
	"github.com/mit-dci/utreexo/util/ttl"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// build the bridge node / proofs
func BuildProofs(
	net wire.BitcoinNet, ttlpath, offsetfile string, sig chan bool) error {

	// Channel to alert the tell the main loop it's ok to exit
	done := make(chan bool, 1)

	// Waitgroup to alert stopBuildProofs() that revoffet and offset has
	// been finished
	offsetFinished := make(chan bool, 1)

	// Channel for stopBuildProofs() to wait
	finish := make(chan bool, 1)

	// Handle user interruptions
	go stopBuildProofs(sig, offsetFinished, done, finish)

	// If given the option testnet=true, check if the blk00000.dat file
	// in the directory is a testnet file. Vise-versa for mainnet
	util.CheckNet(net)

	// Creates all the directories needed for bridgenode
	util.MakePaths()

	// Init forest and variables. Resumes if the data directory exists
	forest, height, knownTipHeight, err :=
		initBridgeNodeState(net, offsetFinished)
	if err != nil {
		panic(err)
	}

	// TEMP only go to block 1000
	knownTipHeight = 390

	// Open leveldb
	o := new(opt.Options)
	o.CompactionTableSizeMultiplier = 8
	lvdb, err := leveldb.OpenFile(ttlpath, o)
	if err != nil {
		panic(err)
	}
	defer lvdb.Close()

	// For ttl value writing
	var batchwg sync.WaitGroup
	batchan := make(chan *leveldb.Batch, 10)

	// Start 16 workers. Just an arbitrary number
	for j := 0; j < 16; j++ {
		go ttl.DbWorker(batchan, lvdb, &batchwg)
	}

	// To send/receive blocks from blockreader()
	blockAndRevReadQueue := make(chan util.BlockAndRev, 10)

	// Reads block asynchronously from .dat files
	// Reads util the lastIndexOffsetHeight
	go util.BlockAndRevReader(blockAndRevReadQueue,
		knownTipHeight, height)
	proofChan := make(chan []byte, 10)
	var fileWait sync.WaitGroup
	go proofWriterWorker(proofChan, &fileWait)

	fmt.Println("Building Proofs and ttldb...")

	var stop bool // bool for stopping the main loop

	for ; height != knownTipHeight && stop != true; height++ {

		// Receive txs from the asynchronous blk*.dat reader
		bnr := <-blockAndRevReadQueue

		// Writes the ttl values for each tx to leveldb
		ttl.WriteBlock(bnr, batchan, &batchwg)

		// Get the add and remove data needed from the block & undo block
		blockAdds, delLeaves, err := genAddDel(bnr)
		if err != nil {
			return err
		}

		// use the accumulator to get inclusion proofs, and produce a block
		// proof with all data needed to verify the block
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

		fmt.Printf("h %d adds %d targets %d\n",
			height, len(blockAdds), len(ud.AccProof.Targets))

		// TODO: Don't ignore undoblock
		// Modifies the forest with the given TXINs and TXOUTs
		_, err = forest.Modify(blockAdds, ud.AccProof.Targets)
		if err != nil {
			return err
		}

		if bnr.Height%10000 == 0 {
			fmt.Println("On block :", bnr.Height+1)
		}

		// Check if stopSig is no longer false
		// stop = true makes the loop exit
		select {
		case stop = <-done: // receives true from stopBuildProofs()
		default:
		}
	}

	// wait until dbWorker() has written to the ttldb file
	// allows leveldb to close gracefully
	batchwg.Wait()

	// Wait for the file workers to finish
	fileWait.Wait()

	// Save the current state so genproofs can be resumed
	err = saveBridgeNodeData(forest, height)
	if err != nil {
		panic(err)
	}

	fmt.Println("Done writing")

	// Tell stopBuildProofs that it's ok to exit
	finish <- true
	return nil

}

/*
Proof file format is somewhat like the blk.dat and rev.dat files.  But it's
always in order!  The offset file is in 8 byte chunks, so to find the proof
data for block 100 (really 101), seek to byte 800 and read 8 bytes.

The proof file is: 4 bytes empty (zeros for now, could do something else later)
4 bytes proof length, then the proof data.

Offset file is: 8 byte int64 offset.  Right now it's all 1 big file, can
change to 4 byte which file and 4 byte offset within file like the blk/rev but
we're not running on fat32 so works OK for now.
*/

// pFileWorker takes in blockproof and height information from the channel
// and writes to disk. MUST NOT have more than one worker as the proofs need to be
// in order
func proofWriterWorker(
	proofChan chan []byte, fileWait *sync.WaitGroup) {

	// for the pFile
	proofFile, err := os.OpenFile(
		util.PFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}

	offsetFile, err := os.OpenFile(
		util.POffsetFilePath, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		panic(err)
	}

	_, err = offsetFile.Seek(0, 2)
	if err != nil {
		panic(err)
	}

	proofFileLocation, err := proofFile.Seek(0, 2)
	if err != nil {
		panic(err)

	}
	// TODO: optimization - don't write anything to proof file for blocks with
	// no deletions (inputs).  Lots of em in testnet.  Not so many on mainnet
	// I guess.  But in testnet would save millions *8 bytes.
	for {
		pbytes := <-proofChan
		// write to offset file first
		err = binary.Write(offsetFile, binary.BigEndian, proofFileLocation)
		if err != nil {
			fmt.Printf(err.Error())
			return
		}

		// write to proof file
		// first write big endian proof size int64
		err = binary.Write(proofFile, binary.BigEndian, int64(len(pbytes)))
		if err != nil {
			fmt.Printf(err.Error())
			return
		}
		proofFileLocation += 8

		// then write the proof
		written, err := proofFile.Write(pbytes)
		if err != nil {
			fmt.Printf(err.Error())
			return
		}
		proofFileLocation += int64(written)

		fileWait.Done()
	}
}

// genBlockProof calls forest.ProveBatch with the hash data to get a batched
// inclusion proof from the accumulator. It then adds on the utxo leaf data,
// to create a block proof which both proves inclusion and gives all utxo data
// needed for transaction verification.
func genUData(delLeaves []util.LeafData, f *accumulator.Forest, height int32) (
	ud util.UData, err error) {

	ud.UtxoData = delLeaves
	// make slice of hashes from leafdata
	delHashes := make([]accumulator.Hash, len(ud.UtxoData))
	for i, _ := range ud.UtxoData {
		delHashes[i] = ud.UtxoData[i].LeafHash()
	}
	// generate block proof. Errors if the tx cannot be proven
	// Should never error out with genproofs as it takes
	// blk*.dat files which have already been vetted by Bitcoin Core
	ud.AccProof, err = f.ProveBatch(delHashes)
	if err != nil {
		err = fmt.Errorf("genBlockProof failed at block %d %s %s",
			height, f.Stats(), err.Error())
		return
	}

	if len(ud.AccProof.Targets) != len(delLeaves) {
		err = fmt.Errorf("genBlockProof %d targets but %d leafData",
			len(ud.AccProof.Targets), len(delLeaves))
		return
	}

	// fmt.Printf(batchProof.ToString())
	// Optional Sanity check. Should never fail.
	unsort := make([]uint64, len(ud.AccProof.Targets))
	copy(unsort, ud.AccProof.Targets)
	ud.AccProof.SortTargets()
	ok := f.VerifyBatchProof(ud.AccProof)
	if !ok {
		return ud, fmt.Errorf("VerifyBatchProof failed at block %d", height)
	}
	ud.AccProof.Targets = unsort

	if !ud.Verify(f.ReconstructStats()) {
		err = fmt.Errorf("height %d LeafData / Proof mismatch", height)
		return
	}
	return
}

// genAddDel is a wrapper around genAdds and genDels. It calls those both and
// throws out all the same block spends.
// It's a little redundant to give back both delLeaves and delHashes, since the
// latter is just the hash of the former, but if we only return delLeaves we
// end up hashing them twice which could slow things down.
func genAddDel(block util.BlockAndRev) (blockAdds []accumulator.Leaf,
	delLeaves []util.LeafData, err error) {

	delLeaves, err = genDels(block)
	if err != nil {
		return
	}

	// this is bridgenode, so don't need to deal with memorable leaves
	blockAdds = util.BlockToAddLeaves(block.Blk, nil, block.Height)
	util.DedupeBlockTxos(&blockAdds, &delLeaves)
	return
}

// genDels generates txs to be deleted from the Utreexo forest. These are TxIns
func genDels(bnr util.BlockAndRev) (
	delLeaves []util.LeafData, err error) {

	// make sure same number of txs and rev txs (minus coinbase)
	if len(bnr.Blk.Transactions)-1 != len(bnr.Rev.Txs) {
		err = fmt.Errorf("genDels block %d %d txs but %d rev txs",
			bnr.Height, len(bnr.Blk.Transactions), len(bnr.Rev.Txs))
		return
	}

	for txinblock, tx := range bnr.Blk.Transactions {
		if txinblock == 0 {
			continue
		}
		txinblock--
		// make sure there's the same number of txins
		if len(tx.TxIn) != len(bnr.Rev.Txs[txinblock].TxIn) {
			err = fmt.Errorf("genDels block %d tx %d has %d inputs but %d rev entries",
				bnr.Height, txinblock+1,
				len(tx.TxIn), len(bnr.Rev.Txs[txinblock].TxIn))
			return
		}
		// loop through inputs
		for i, txin := range tx.TxIn {
			// build leaf
			var l util.LeafData

			l.Outpoint = txin.PreviousOutPoint
			l.Height = bnr.Rev.Txs[txinblock].TxIn[i].Height
			l.Coinbase = bnr.Rev.Txs[txinblock].TxIn[i].Coinbase
			// TODO get blockhash from headers here -- empty for now
			// l.BlockHash = getBlockHashByHeight(l.CbHeight >> 1)
			l.Amt = bnr.Rev.Txs[txinblock].TxIn[i].Amount
			l.PkScript = bnr.Rev.Txs[txinblock].TxIn[i].PKScript
			delLeaves = append(delLeaves, l)
		}
	}

	return
}

// stopBuildProofs listens for the signal from the OS and initiates an exit squence
func stopBuildProofs(
	sig, offsetfinished, done, finish chan bool) {

	// Listen for SIGINT, SIGQUIT, SIGTERM
	<-sig

	// Sometimes there are bugs that make the program run forver.
	// Utreexo binary should never take more than 10 seconds to exit
	go func() {
		time.Sleep(10 * time.Second)
		fmt.Println("Program timed out. Force quitting. Data likely corrupted")
		os.Exit(1)
	}()

	// Tell the user that the sig is received
	fmt.Println("User exit signal received. Exiting...")

	select {
	// If offsetfile is there or was built, don't remove it
	case <-offsetfinished:
		select {
		default:
			done <- true
		}
	// If nothing is received, delete offsetfile and other directories
	// Don't wait for done channel from the main BuildProofs() for loop
	default:
		select {
		default:
			fmt.Println("offsetfile incomplete, removing...")
			// May not work sometimes.
			err := os.RemoveAll(util.OffsetDirPath)
			if err != nil {
				fmt.Println("ERR. offsetdata/ directory not removed. Please manually remove it.")
			}
			err = os.RemoveAll(util.RevOffsetDirPath)
			if err != nil {
				fmt.Println("ERR. revdata/ directory not removed. Please manually remove it.")
			}
			fmt.Println("Exiting...")
			os.Exit(0)
		}
	}

	// Wait until BuildProofs() or buildOffsetFile() says it's ok to exit
	<-finish
	os.Exit(0)
}
