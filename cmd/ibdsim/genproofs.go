package ibdsim

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/mit-dci/lit/wire"
	"github.com/mit-dci/utreexo/cmd/simutil"
	"github.com/mit-dci/utreexo/utreexo"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// build the bridge node / proofs
func BuildProofs(
	isTestnet bool, ttldb string, offsetfile string, sig chan bool) error {

	// Channel to alert the main loop to break
	stopGoing := make(chan bool, 1)

	// Channel to alert stopTxottl it's ok to exit
	done := make(chan bool, 1)

	// Channel to alert stopBuildProofs() that buildOffsetFile() has been finished
	offsetfinished := make(chan bool, 1)

	// Channel for stopBuildProofs() to wait
	finish := make(chan bool, 1)

	// Handle user interruptions
	go stopBuildProofs(sig, offsetfinished, stopGoing, done, finish, offsetfile)

	//defaults to the testnet Gensis tip
	tip := simutil.TestNet3GenHash

	//If given the option testnet=true, check if the blk00000.dat file
	//in the directory is a testnet file. Vise-versa for mainnet
	simutil.CheckTestnet(isTestnet)

	if isTestnet != true {
		tip = simutil.MainnetGenHash
	}

	// Creates all the paths needed for simcmd
	simutil.MakePaths()

	var currentOffsetHeight int
	height := 0
	nextMap := make(map[[32]byte]simutil.RawHeaderData)

	// if there isn't an offset file, make one
	if simutil.HasAccess(simutil.OffsetFilePath) == false {
		fmt.Println("offsetfile not present. Building...")
		currentOffsetHeight, _ = buildOffsetFile(tip, height, nextMap,
			simutil.OffsetFilePath, simutil.CurrentOffsetFilePath, offsetfinished)
	} else {
		// if there is a offset file, we should pass true to offsetfinished
		// to let stopParse() know that it shouldn't delete offsetfile
		offsetfinished <- true
	}

	//if there is a heightfile, get the height from that
	// heightFile saves the last block that was written to ttldb
	var err error
	if simutil.HasAccess(simutil.HeightFilePath) {
		heightFile, err := os.OpenFile(
			simutil.HeightFilePath, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			panic(err)
		}
		var t [4]byte
		_, err = heightFile.Read(t[:])
		if err != nil {
			return err
		}
		height = int(simutil.BtU32(t[:]))
	}
	heightFile, err := os.OpenFile(
		simutil.HeightFilePath, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}

	// grab the last block height from currentoffsetheight
	// currentoffsetheight saves the last height from the offsetfile
	var currentOffsetHeightByte [4]byte
	currentOffsetHeightFile, err := os.OpenFile(
		simutil.CurrentOffsetFilePath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		panic(err)
	}
	_, err = currentOffsetHeightFile.Read(currentOffsetHeightByte[:])
	if err != nil {
		panic(err)
	}
	currentOffsetHeightFile.Read(currentOffsetHeightByte[:])
	currentOffsetHeight = int(simutil.BtU32(currentOffsetHeightByte[:]))

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

	//start db writer worker... actually start a bunch of em
	// try 16 workers...?
	for j := 0; j < 16; j++ {
		go dbWorker(batchan, lvdb, &batchwg)
	}

	// Where the proofs for txs exist
	pFile, err := os.OpenFile(
		simutil.PFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer pFile.Close()

	// Gives the location of where a particular block height's proofs are
	// Basically an index
	var pOffset uint32
	if simutil.HasAccess(simutil.POffsetCurrentOffsetFilePath) {
		pOffsetCurrentOffsetFile, err := os.OpenFile(
			simutil.POffsetCurrentOffsetFilePath,
			os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			panic(err)
		}
		pOffset, err = simutil.GetPOffsetNum(pOffsetCurrentOffsetFile)
		if err != nil {
			panic(err)
		}
		fmt.Println("Poffset restored to", pOffset)

	}

	pOffsetFile, err := os.OpenFile(
		simutil.POffsetFilePath,
		os.O_APPEND|os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		panic(err)
	}
	pOffsetCurrentOffsetFile, err := os.OpenFile(
		simutil.POffsetCurrentOffsetFilePath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		panic(err)
	}

	var newForest *utreexo.Forest
	if simutil.HasAccess(simutil.ForestFilePath) {
		fmt.Println("forestFile access")

		// Where the forestfile exists
		forestFile, err := os.OpenFile(
			simutil.ForestFilePath, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return err
		}

		// Other forest variables
		miscForestFile, err := os.OpenFile(
			simutil.MiscForestFilePath, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return err
		}

		// Restores all the forest data
		newForest, err = utreexo.RestoreForest(miscForestFile, forestFile)
		if err != nil {
			panic(err)
		}
	} else {
		fmt.Println("No forestFile access")
		forestFile, err := os.OpenFile(simutil.ForestFilePath, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return err
		}
		newForest = utreexo.NewForest(forestFile)
	}

	// Other forest variables
	miscForestFile, err := os.OpenFile(
		simutil.MiscForestFilePath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}

	var totalProofNodes int

	// bool for stopping the main loop
	var stop bool

	// To send/receive blocks from blockreader()
	bchan := make(chan simutil.BlockToWrite, 10)

	fmt.Println("Building Proofs and ttldb...")

	// Reads block asynchronously from .dat files
	go simutil.BlockReader(bchan, currentOffsetHeight, height, simutil.OffsetFilePath)

	for ; height != currentOffsetHeight && stop != true; height++ {

		b := <-bchan

		err := writeProofs(b.Txs, b.Height,
			pFile, pOffsetFile, newForest, totalProofNodes, &pOffset)
		if err != nil {
			panic(err)
		}

		err = writeBlock(b.Txs, b.Height+1, batchan, &batchwg)
		if err != nil {
			panic(err)
		}

		if b.Height%10000 == 0 {
			fmt.Println("On block :", b.Height+1)
		}

		// Check if stopSig is no longer false
		// stop = true makes the loop exit
		select {
		case stop = <-done:
		default:
		}
	}

	fmt.Println("Cleaning up for exit...")

	// write to the heightfile
	_, err = heightFile.WriteAt(simutil.U32tB(uint32(height)), 0)
	if err != nil {
		panic(err)
	}
	heightFile.Close()

	err = newForest.WriteForest(miscForestFile)
	if err != nil {
		panic(err)
	}
	_, err = pOffsetCurrentOffsetFile.WriteAt(
		simutil.U32tB(pOffset), 0)
	if err != nil {
		panic(err)
	}

	// wait until dbWorker() has written to the ttldb file
	// allows leveldb to close gracefully
	batchwg.Wait()

	fmt.Println("Poffset is", pOffset)

	fmt.Println("Done writing")

	// Tell stopBuildProofs that it's ok to exit
	finish <- true
	return nil

}

//Here we write proofs for all the txs.
//All the inputs are saved as 32byte sha256 hashes.
//All the outputs are saved as LeafTXO type.
func writeProofs(
	tx []*wire.MsgTx,
	height int,
	pFile *os.File,
	pOffsetFile *os.File,
	newForest *utreexo.Forest,
	totalProofNodes int,
	pOffset *uint32) error {

	var blockAdds []utreexo.LeafTXO
	var blockDels []utreexo.Hash
	var plustime time.Duration

	blocktxs := []*simutil.Txotx{new(simutil.Txotx)}
	plusstart := time.Now()

	for blockindex, tx := range tx {
		for _, in := range tx.TxIn {
			if blockindex > 0 { // skip coinbase "spend"
				opString := in.PreviousOutPoint.String()
				h := utreexo.HashFromString(opString)
				blockDels = append(blockDels, h)
			}
		}

		//creates all txos up to index indicated
		txhash := tx.TxHash()
		numoutputs := uint32(len(tx.TxOut))

		blocktxs[len(blocktxs)-1].Unspendable = make([]bool, numoutputs)
		//Adds z and index for all OP_RETURN transactions
		//We don't keep track of the OP_RETURNS so probably can get rid of this
		for i, out := range tx.TxOut {
			if simutil.IsUnspendable(out) {
				// Skip all the unspendables
				blocktxs[len(blocktxs)-1].Unspendable[i] = true
			} else {
				//txid := tx.TxHash().String()
				blocktxs[len(blocktxs)-1].Outputtxid = txhash.String()
			}
		}
		// done with this txotx, make the next one and append
		blocktxs = append(blocktxs, new(simutil.Txotx))

	}
	//TODO Get rid of this. This eats up cpu
	//we started a tx but shouldn't have
	blocktxs = blocktxs[:len(blocktxs)-1]

	for _, b := range blocktxs {
		adds, err := hashgen(b)
		if err != nil {
			return err
		}
		for _, add := range adds {
			blockAdds = append(blockAdds, add)
		}
	}

	donetime := time.Now()
	plustime += donetime.Sub(plusstart)

	//Forget all utxos that get spent on the same block
	//they are created.
	utreexo.DedupeHashSlices(&blockAdds, &blockDels)

	blockProof, err := newForest.ProveBlock(blockDels)
	if err != nil {
		return fmt.Errorf("ProveBlock failed at block %d %s %s", height+1, newForest.Stats(), err.Error())
	}

	ok := newForest.VerifyBlockProof(blockProof)
	if !ok {
		return fmt.Errorf("VerifyBlockProof failed at block %d", height+1)
	}

	totalProofNodes += len(blockProof.Proof)

	// U32tB always returns 4 bytes
	// Later this could also be changed to magic bytes
	_, err = pFile.Write(utreexo.U32tB(uint32(height + 1)))
	if err != nil {
		return err
	}
	p := blockProof.ToBytes()

	// write the offset for a block
	_, err = pOffsetFile.Write(utreexo.U32tB(*pOffset))
	if err != nil {
		return err
	}
	*pOffset += uint32(len(p)) + uint32(8) // add 8 for height bytes and load size
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
	_, err = newForest.Modify(blockAdds, blockProof.Targets)
	if err != nil {
		return err
	}
	// empty the blockAdds and blockDels that were written
	blockAdds = []utreexo.LeafTXO{}
	blockDels = []utreexo.Hash{}

	return nil
}

func hashgen(tx *simutil.Txotx) ([]utreexo.LeafTXO, error) {
	adds := []utreexo.LeafTXO{}
	for i := 0; i < len(tx.Unspendable); i++ {
		if tx.Unspendable[i] {
			continue
		}
		utxostring := fmt.Sprintf("%s;%d", tx.Outputtxid, i)
		addData := utreexo.LeafTXO{Hash: utreexo.HashFromString(utxostring)}
		adds = append(adds, addData)
	}
	return adds, nil
}

func stopBuildProofs(
	sig, offsetfinished, stopGoing, done, finish chan bool, offsetfile string) {

	<-sig

	//Tell BuildProofs to finish the block it's working on
	stopGoing <- true
	select {
	//If offsetfile is there or was built, don't remove it
	case <-offsetfinished:
		select {
		default:
			done <- true
		}
	//If nothing is received, delete offsetfile and currentoffsetheight
	default:
		select {
		default:
			os.Remove(offsetfile)
			os.Remove("currentoffsetheight")
			fmt.Println("offsetfile incomplete, removing...")
			done <- true
		}
	}

	// Wait until BuildProofs() says it's ok to exit
	<-finish

	fmt.Println("Exiting...")
	os.Exit(0)
}
