package ibdsim

import (
	"fmt"
	"os"
	"time"

	"github.com/mit-dci/lit/wire"
	"github.com/mit-dci/utreexo/cmd/utils"
	"github.com/mit-dci/utreexo/utreexo"
)

// build the bridge node / proofs
func BuildProofs(isTestnet bool, offsetfile string, sig chan bool) error {

	//Channel to alert the main loop to break
	stopGoing := make(chan bool, 1)

	//Channel to alert stopTxottl it's ok to exit
	done := make(chan bool, 1)

	go stopBuildProofs(sig, stopGoing, done)

	//Check if it was ran inside testnet directory
	simutil.CheckTestnet(isTestnet)

	//fmt.Println(ttlfn)
	//txofile, err := os.OpenFile(ttlfn, os.O_RDONLY, 0600)
	//if err != nil {
	//	return err
	//}

	var currentOffsetHeight int
	//grab the last block height from currentoffsetheight
	//currentoffsetheight saves the last height from the offsetfile
	var currentOffsetHeightByte [4]byte
	currentOffsetHeightFile, err := os.Open("currentoffsetheight")
	if err != nil {
		panic(err)
	}
	currentOffsetHeightFile.Read(currentOffsetHeightByte[:])
	currentOffsetHeight = int(simutil.BtU32(currentOffsetHeightByte[:]))

	pFile, err := os.OpenFile("proof.dat", os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	pOffsetFile, err := os.OpenFile("proofoffset.dat", os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	//proofDB, err := leveldb.OpenFile("./proofdb", nil)
	//if err != nil {
	//	return err
	//}
	offsetFile, err := os.Open(offsetfile)
	if err != nil {
		panic(err)
	}
	newForest := utreexo.NewForest()

	var height uint32
	var totalProofNodes int
	var pOffset uint32
	//starttime := time.Now()

	//bool for stopping the scanner.Scan loop
	var stop bool

	fmt.Println("Building Proofs...")

	for ; int(height) != currentOffsetHeight && stop != true; height++ {
		block, err := simutil.GetRawBlockFromFile(int(height), offsetFile)
		if err != nil {
			panic(err)
		}

		err = writeProofs(block, int(height), pFile, pOffsetFile,
			newForest, totalProofNodes, &pOffset)
		if err != nil {
			panic(err)
		}

		//if height%10000 == 0 {
		//	fmt.Printf("Block %d %s plus %.2f total %.2f proofnodes %d \n",
		//		height, newForest.Stats(),
		//		plustime.Seconds(), time.Now().Sub(starttime).Seconds(),
		//		totalProofNodes)
		//}

		//Check if stopSig is no longer false
		//stop = true makes the loop exit
		select {
		case stop = <-stopGoing:
		default:
		}

	}
	fmt.Println("Done writing")
	pFile.Close()
	pOffsetFile.Close()
	// Tell stopBuildProofs that it's ok to exit
	done <- true
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
		return fmt.Errorf("block %d %s %s", height, newForest.Stats(), err.Error())
	}

	ok := newForest.VerifyBlockProof(blockProof)
	if !ok {
		return fmt.Errorf("VerifyBlockProof failed at block %d", height)
	}

	totalProofNodes += len(blockProof.Proof)

	//U32tB always returns 4 bytes
	//Later this could also be changed to magic bytes
	_, err = pFile.Write(utreexo.U32tB(uint32(height)))
	if err != nil {
		return err
	}
	p := blockProof.ToBytes()

	//write the offset for a block
	_, err = pOffsetFile.Write(utreexo.U32tB(*pOffset))
	if err != nil {
		return err
	}
	*pOffset += uint32(len(p)) + uint32(8) // add 4 for height bytes
	//write the size of the proof
	_, err = pFile.Write(utreexo.U32tB(uint32(len(p))))
	if err != nil {
		return err
	}
	//Write the actual proof
	_, err = pFile.Write(p)
	if err != nil {
		return err
	}
	_, err = newForest.Modify(blockAdds, blockProof.Targets)
	if err != nil {
		return err
	}
	//empty the blockAdds and blockDels that were written
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

func stopBuildProofs(sig chan bool, stopGoing chan bool, done chan bool) {
	<-sig

	//Tell BuildProofs to finish the block it's working on
	stopGoing <- true

	//Wait until BuildProofs says it's ok to quit
	<-done

	fmt.Println("Exiting...")
	os.Exit(0)
}
