package bridge

import (
	"bufio"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/btcsuite/btcd/wire"
	"github.com/mit-dci/utreexo/cmd/ttl"
	"github.com/mit-dci/utreexo/cmd/util"
	"github.com/mit-dci/utreexo/utreexo"
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
	blockReadQueue := make(chan util.BlockAndRev, 10)

	// Reads block asynchronously from .dat files
	// Reads util the lastIndexOffsetHeight
	go util.BlockReader(
		blockReadQueue, lastIndexOffsetHeight, height,
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
		block := <-blockReadQueue

		// Writes the ttl values for each tx to leveldb
		ttl.WriteBlock(block, batchan, &batchwg)

		// Get the add and remove data needed from the block & undo block
		blockAdds, delLeaves, delHashes, err := genAddDel(block)
		if err != nil {
			return err
		}

		// use the accumulator to get inclusion proofs, and produce a block
		// proof with all data needed to verify the block
		blkProof, err := genBlockProof(delLeaves, delHashes, forest, block.Height)
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
			Proof: b, Height: block.Height}

		// TODO: Don't ignore undoblock
		// Modifies the forest with the given TXINs and TXOUTs
		_, err = forest.Modify(blockAdds, blkProof.Proof.Targets)
		if err != nil {
			return err
		}

		if block.Height%10000 == 0 {
			fmt.Println("On block :", block.Height+1)
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
			utreexo.U32tB(uint32(bp.Height+1))...)

		// write the size of the proof
		writebyte = append(writebyte,
			utreexo.U32tB(uint32(len(bp.Proof)))...)

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
func genBlockProof(dels []util.LeafData, delHashes []utreexo.Hash,
	f *utreexo.Forest, height int32) (
	util.BlockProof, error) {

	var blockP util.BlockProof
	// generate block proof. Errors if the tx cannot be proven
	// Should never error out with genproofs as it takes
	// blk*.dat files which have already been vetted by Bitcoin Core
	batchProof, err := f.ProveBatch(delHashes)
	if err != nil {
		return blockP, fmt.Errorf("ProveBlock failed at block %d %s %s",
			height+1, f.Stats(), err.Error())
	}

	// Takes time and redundant, can enable for testing but not real use
	// Sanity check. Should never fail.
	// ok := f.VerifyBatchProof(blockProof)
	// if !ok {
	// return blockProof,
	// fmt.Errorf("VerifyBlockProof failed at block %d", height+1)
	// }

	// Add leafData to struct here

	return blockP, nil
}

// genAddDel is a wrapper around genAdds and genDels. It calls those both and
// throws out all the same block spends.
// It's a little redundant to give back both delLeaves and delHashes, since the
// latter is just the hash of the former, but if we only return delLeaves we
// end up hashing them twice which could slow things down.
func genAddDel(block util.BlockAndRev) (blockAdds []utreexo.LeafTXO,
	delLeaves []util.LeafData, delHashes []utreexo.Hash, err error) {

	delLeaves, delHashes, err = genDels(block)
	blockAdds = genAdds(block)

	utreexo.DedupeHashSlices(&blockAdds, &delHashes)
	return
}

// genAdds generates leafTXOs to be added to the Utreexo forest. These are TxOuts
// Skips all the OP_RETURN transactions
func genAdds(bl util.BlockAndRev) (hashleaves []utreexo.LeafTXO) {
	bh := bl.Blockhash
	cheight := bl.Height << 1 // *2 because of the weird coinbase bit thing
	for coinbaseif0, tx := range bl.Txs {
		// cache txid aka txhash
		txid := tx.MsgTx().TxHash()
		for i, out := range tx.MsgTx().TxOut {
			// Skip all the OP_RETURNs
			if util.IsUnspendable(out) {
				continue
			}
			var l util.LeafData
			// TODO put blockhash back in -- leaving empty for now!
			// l.BlockHash = bh
			l.Outpoint.Hash = txid
			l.Outpoint.Index = uint32(i)
			l.CbHeight = cheight
			if coinbaseif0 == 0 {
				l.CbHeight |= 1
			}
			l.Amt = out.Value
			l.PkScript = out.PkScript

			// Don't need to save leafData here
			// dataleaves = append(dataleaves, l)

			var uleaf utreexo.LeafTXO
			uleaf.Hash = l.LeafHash()
			hashleaves = append(hashleaves, uleaf)
		}
	}
	return
}

// genDels generates txs to be deleted from the Utreexo forest. These are TxIns
func genDels(block util.BlockAndRev) (
	delLeaves []util.LeafData, delHashes []utreexo.Hash, err error) {

	// make sure same number of txs and rev txs
	if len(block.Txs) != len(block.Rev.Txs) {
		err = fmt.Errorf("block %d %d txs but %d rev txs",
			block.Height, len(block.Txs), len(block.Rev.Txs))
	}
	for txinblock, tx := range block.Txs {
		if txinblock == 0 {
			continue
		}
		// make sure there's the same number of txins
		if len(tx.MsgTx().TxIn) != len(block.Rev.Txs[txinblock].TxIn) {
			err = fmt.Errorf("block %d tx %d has %d inputs but %d rev entries",
				block.Height, txinblock+1,
				len(tx.MsgTx().TxIn), len(block.Rev.Txs[txinblock].TxIn))
		}
		// loop through inputs
		for i, txin := range tx.MsgTx().TxIn {
			// build leaf
			var l util.LeafData

			l.Outpoint = txin.PreviousOutPoint
			l.CbHeight = block.Rev.Txs[txinblock].TxIn[i].Height
			// TODO get blockhash from headers here -- empty for now
			// l.BlockHash = getBlockHashByHeight(l.CbHeight >> 1)

			if txinblock == 0 {
				l.CbHeight |= 1
			}
			l.Amt = out.Value
			l.PkScript = out.PkScript

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
