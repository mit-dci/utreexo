package bridgenode

import (
	"bufio"
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
	forest, height, lastIndexOffsetHeight, pOffset, err :=
		initBridgeNodeState(net, offsetFinished)
	if err != nil {
		panic(err)
	}

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
		lastIndexOffsetHeight, height,
		util.OffsetFilePath, util.RevOffsetFilePath)

	// for the pFile
	proofAndHeightChan := make(chan util.ProofAndHeight, 1)
	pFile, err := os.OpenFile(
		util.PFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}
	pFileBuf := bufio.NewWriter(pFile) // buffered write to file

	// for pOffsetFile
	proofChan := make(chan []byte, 1)
	pOffsetFile, err := os.OpenFile(
		util.POffsetFilePath, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		panic(err)
	}
	pOffsetFileBuf := bufio.NewWriter(pOffsetFile) // buffered write to file

	var fileWait sync.WaitGroup
	go pFileWorker(proofAndHeightChan, pFileBuf, &fileWait, done)
	go pOffsetFileWorker(proofChan, &pOffset, pOffsetFileBuf, &fileWait, done)

	fmt.Println("Building Proofs and ttldb...")

	var stop bool // bool for stopping the main loop

	for ; height != lastIndexOffsetHeight && stop != true; height++ {

		// Receive txs from the asynchronous blk*.dat reader
		bnr := <-blockAndRevReadQueue

		// Writes the ttl values for each tx to leveldb
		ttl.WriteBlock(bnr, batchan, &batchwg)

		// Get the add and remove data needed from the block & undo block
		blockAdds, delLeaves, delHashes, err := genAddDel(bnr)
		if err != nil {
			return err
		}

		// use the accumulator to get inclusion proofs, and produce a block
		// proof with all data needed to verify the block
		blkProof, err := genBlockProof(delLeaves, delHashes, forest, bnr.Height)
		if err != nil {
			return err
		}

		// convert blockproof struct to bytes
		b := blkProof.ToBytes()

		// Add to WaitGroup and send data to channel to be written
		// to disk
		fileWait.Add(1)
		proofChan <- b

		// Add to WaitGroup and send data to channel to be written
		// to disk
		fileWait.Add(1)
		proofAndHeightChan <- util.ProofAndHeight{
			Proof: b, Height: bnr.Height}

		// TODO: Don't ignore undoblock
		// Modifies the forest with the given TXINs and TXOUTs
		_, err = forest.Modify(blockAdds, blkProof.AccProof.Targets)
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
	pFileBuf.Flush()
	pOffsetFileBuf.Flush()

	// Save the current state so genproofs can be resumed
	err = saveBridgeNodeData(forest, pOffset, height)
	if err != nil {
		panic(err)
	}

	fmt.Println("Done writing")

	// Tell stopBuildProofs that it's ok to exit
	finish <- true
	return nil

}

// pFileWorker takes in blockproof and height information from the channel
// and writes to disk. MUST NOT have more than one worker as the proofs need to be
// in order
func pFileWorker(blockProofAndHeight chan util.ProofAndHeight,
	pFileBuf *bufio.Writer, fileWait *sync.WaitGroup, done chan bool) {

	for {

		bp := <-blockProofAndHeight

		var writebyte []byte
		// U32tB always returns 4 bytes
		// Later this could also be changed to magic bytes
		writebyte = append(writebyte,
			util.U32tB(uint32(bp.Height+1))...)

		// write the size of the proof
		writebyte = append(writebyte,
			util.U32tB(uint32(len(bp.Proof)))...)

		// Write the actual proof
		writebyte = append(writebyte, bp.Proof...)

		_, err := pFileBuf.Write(writebyte)
		if err != nil {
			panic(err)
		}
		fileWait.Done()
	}
}

// pOffsetFileWorker receives proofs from the channel and writes to disk
// aynschornously. MUST NOT have more than one worker as the proofoffsets need to be
// in order.
func pOffsetFileWorker(proofChan chan []byte, pOffset *int32,
	pOffsetFileBuf *bufio.Writer, fileWait *sync.WaitGroup, done chan bool) {

	for {
		bp := <-proofChan

		var writebyte []byte
		writebyte = append(writebyte, util.I32tB(*pOffset)...)

		// Updates the global proof offset. Need for resuming purposes
		*pOffset += int32(len(bp)) + int32(8) // add 8 for height bytes and load size

		_, err := pOffsetFileBuf.Write(writebyte)
		if err != nil {
			panic(err)
		}

		fileWait.Done()
	}

}

// genBlockProof calls forest.ProveBatch with the hash data to get a batched
// inclusion proof from the accumulator.  It then adds on the utxo leaf data,
// to create a block proof which both proves inclusion and gives all utxo data
// needed for transaction verification.
func genBlockProof(delLeaves []util.LeafData, delHashes []accumulator.Hash,
	f *accumulator.Forest, height int32) (
	util.UData, error) {

	var blockP util.UData
	// generate block proof. Errors if the tx cannot be proven
	// Should never error out with genproofs as it takes
	// blk*.dat files which have already been vetted by Bitcoin Core
	batchProof, err := f.ProveBatch(delHashes)
	if err != nil {
		return blockP, fmt.Errorf("ProveBlock failed at block %d %s %s",
			height+1, f.Stats(), err.Error())
	}
	if len(batchProof.Targets) != len(delLeaves) {
		return blockP, fmt.Errorf("ProveBatch %d targets but %d leafData",
			len(batchProof.Targets), len(delLeaves))
	}

	// Optional Sanity check. Should never fail.
	// ok := f.VerifyBatchProof(blockProof)
	// if !ok {
	// return blockProof,
	// fmt.Errorf("VerifyBlockProof failed at block %d", height+1)
	// }

	blockP.AccProof = batchProof
	blockP.UtxoData = delLeaves

	return blockP, nil
}

// genAddDel is a wrapper around genAdds and genDels. It calls those both and
// throws out all the same block spends.
// It's a little redundant to give back both delLeaves and delHashes, since the
// latter is just the hash of the former, but if we only return delLeaves we
// end up hashing them twice which could slow things down.
func genAddDel(block util.BlockAndRev) (blockAdds []accumulator.Leaf,
	delLeaves []util.LeafData, delHashes []accumulator.Hash, err error) {

	delLeaves, delHashes, err = genDels(block)
	if err != nil {
		return
	}
	blockAdds = util.BlockToAdds(block.Blk, block.Height)

	accumulator.DedupeHashSlices(&blockAdds, &delHashes)
	return
}

// genDels generates txs to be deleted from the Utreexo forest. These are TxIns
func genDels(bnr util.BlockAndRev) (
	delLeaves []util.LeafData, delHashes []accumulator.Hash, err error) {

	// make sure same number of txs and rev txs (minus coinbase)
	if len(bnr.Blk.Transactions)-1 != len(bnr.Rev.Txs) {
		err = fmt.Errorf("block %d %d txs but %d rev txs",
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
			err = fmt.Errorf("block %d tx %d has %d inputs but %d rev entries",
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
			// TODO get blockhash from headers here -- empty for now
			// l.BlockHash = getBlockHashByHeight(l.CbHeight >> 1)

			if txinblock == 0 {
				l.Coinbase = true
			}
			l.Amt = bnr.Rev.Txs[txinblock].TxIn[i].Amount
			l.PkScript = bnr.Rev.Txs[txinblock].TxIn[i].PKScript
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
