package bridge

import (
	"fmt"
	"os"
	"sync"

	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"github.com/mit-dci/utreexo/cmd/ttl"
	"github.com/mit-dci/utreexo/cmd/util"
	"github.com/mit-dci/utreexo/utreexo"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// build the bridge node / proofs
func BuildProofs(
	net wire.BitcoinNet, ttldb string, offsetfile string, sig chan bool) error {

	// Channel to alert the tell the main loop it's ok to exit
	done := make(chan bool, 1)

	// Channel to alert stopBuildProofs() that buildOffsetFile() has been finished
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
	lvdb, err := leveldb.OpenFile(ttldb, o)
	if err != nil {
		panic(err)
	}
	defer lvdb.Close()

	var batchwg sync.WaitGroup
	// make the channel ... have a buffer? does it matter?
	batchan := make(chan *leveldb.Batch)

	// start db writer worker... actually start a bunch of em
	// try 16 workers...?
	for j := 0; j < 16; j++ {
		go ttl.DbWorker(batchan, lvdb, &batchwg)
	}

	fmt.Println("Building Proofs and ttldb...")

	// To send/receive blocks from blockreader()
	txChan := make(chan util.TxToWrite, 10)

	// Reads block asynchronously from .dat files
	go util.BlockReader(
		txChan, lastIndexOffsetHeight, height, util.OffsetFilePath)

	// Where the proofs for txs exist
	pFile, err := os.OpenFile(
		util.PFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}
	pOffsetFile, err := os.OpenFile(
		util.POffsetFilePath, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		panic(err)
	}

	var stop bool // bool for stopping the main loop

	for ; height != lastIndexOffsetHeight && stop != true; height++ {

		// Receive txs from the asynchronous blk*.dat reader
		txs := <-txChan

		err = writeProofs(txs.Txs, txs.Height,
			pFile, pOffsetFile, forest, &pOffset)
		if err != nil {
			panic(err)
		}

		// Save the ttl values to leveldb
		err = ttl.WriteBlock(txs.Txs, txs.Height+1, batchan, &batchwg)
		if err != nil {
			panic(err)
		}

		if txs.Height%10000 == 0 {
			fmt.Println("On block :", txs.Height+1)
		}

		// Check if stopSig is no longer false
		// stop = true makes the loop exit
		select {
		case stop = <-done: // receives true from stopBuildProofs()
		default:
		}
	}
	pFile.Close()
	pOffsetFile.Close()

	// wait until dbWorker() has written to the ttldb file
	// allows leveldb to close gracefully
	batchwg.Wait()

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

// writeProofs writes the proofs to the given pFile and modifies the Utreexo
// Forest. Main function for generating proofs
func writeProofs(txs []*btcutil.Tx, height int32, pFile *os.File, pOffsetFile *os.File,
	forest *utreexo.Forest, pOffset *int32) error {

	// generate the adds and dels in LeafTXO format
	blockAdds, blockDels, err := genAddDel(txs)

	// generate block proof. Errors if the tx cannot be proven
	// Should never error out with genproofs
	blockProof, err := forest.ProveBlock(blockDels)
	if err != nil {
		return fmt.Errorf("ProveBlock failed at block %d %s %s", height+1, forest.Stats(), err.Error())
	}

	// Sanity check. Don't really need it for now
	ok := forest.VerifyBlockProof(blockProof)
	if !ok {
		return fmt.Errorf("VerifyBlockProof failed at block %d", height+1)
	}

	// U32tB always returns 4 bytes
	// Later this could also be changed to magic bytes
	_, err = pFile.Write(utreexo.U32tB(uint32(height + 1)))
	if err != nil {
		return err
	}

	// convert blockproof struct to bytes
	p := blockProof.ToBytes()

	// write the offset for a block
	_, err = pOffsetFile.Write(util.I32tB(*pOffset))
	if err != nil {
		return err
	}

	// Updates the global proof offset. Need for resuming purposes
	*pOffset += int32(len(p)) + int32(8) // add 8 for height bytes and load size

	// write the size of the proof
	_, err = pFile.Write(utreexo.U32tB(uint32(len(p))))
	if err != nil {
		return err
	}
	// Write the actual proof
	_, err = pFile.Write(p)
	if err != nil {
		return err
	}

	// Modify the forest
	_, err = forest.Modify(blockAdds, blockProof.Targets)
	if err != nil {
		return err
	}
	return nil
}

// genAddDel takes in a raw Bitcoin message tx and outputs a
// slice of LeafTXOs to be added and a slice of hashes to be
// deleted.
func genAddDel(txs []*btcutil.Tx) (
	blockAdds []utreexo.LeafTXO, blockDels []utreexo.Hash, err error) {

	blocktxs := []*util.Txotx{new(util.Txotx)}

	for blockindex, tx := range txs {
		for _, in := range tx.MsgTx().TxIn {
			if blockindex > 0 { // skip coinbase "spend"
				opString := in.PreviousOutPoint.String()
				// TODO replace with TXID and index
				h := utreexo.HashFromString(opString)
				blockDels = append(blockDels, h)
			}
		}
		// Set txid aka txhash
		blocktxs[len(blocktxs)-1].Outputtxid = tx.MsgTx().TxHash().String()

		// creates all txos up to index indicated
		numoutputs := uint32(len(tx.MsgTx().TxOut))

		// For tagging each TXO as unspendable
		blocktxs[len(blocktxs)-1].Unspendable = make([]bool, numoutputs)
		for i, out := range tx.MsgTx().TxOut {
			if util.IsUnspendable(out) {
				// Tag unspendable
				blocktxs[len(blocktxs)-1].Unspendable[i] = true
			}
		}
		// done with this txotx, make the next one and append
		blocktxs = append(blocktxs, new(util.Txotx))

	}
	// TODO Get rid of this. This eats up cpu
	// we started a tx but shouldn't have
	blocktxs = blocktxs[:len(blocktxs)-1]

	for _, b := range blocktxs {
		adds, err := genLeafTXO(b)
		if err != nil {
			return nil, nil, err
		}
		for _, add := range adds {
			blockAdds = append(blockAdds, add)
		}
	}

	// Forget all utxos that get spent on the same block
	// they are created.
	utreexo.DedupeHashSlices(&blockAdds, &blockDels)
	return
}

// genLeafTXO takes in txs from a block and contructs a slice
// of LeafTXOs ready to be processed by the utreexo library
// skips all OP_RETURN transactions
func genLeafTXO(tx *util.Txotx) ([]utreexo.LeafTXO, error) {
	adds := []utreexo.LeafTXO{}
	for i := 0; i < len(tx.Unspendable); i++ {
		if tx.Unspendable[i] {
			continue
		}
		utxostring := fmt.Sprintf("%s:%d", tx.Outputtxid, i)
		addData := utreexo.LeafTXO{Hash: utreexo.HashFromString(utxostring)}
		adds = append(adds, addData)
	}
	return adds, nil
}

func stopBuildProofs(
	sig, offsetfinished, done, finish chan bool) {

	// Listen for SIGINT, SIGQUIT, SIGTERM
	<-sig

	select {
	// If offsetfile is there or was built, don't remove it
	case <-offsetfinished:
		select {
		default:
			done <- true
		}
	// If nothing is received, delete offsetfile and other directories
	default:
		select {
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
	}

	// Wait until BuildProofs() or buildOffsetFile() says it's ok to exit
	<-finish

	fmt.Println("Exiting...")
	os.Exit(0)
}
