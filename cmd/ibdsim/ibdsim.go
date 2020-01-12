/* test the utreexo forest */

package ibdsim

import (
	"fmt"
	"os"
	"time"

	"github.com/mit-dci/lit/wire"
	"github.com/mit-dci/utreexo/cmd/utils"
	"github.com/mit-dci/utreexo/utreexo"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

var maxmalloc uint64

// run IBD from block proof data
// we get the new utxo info from the same txos text file
// the deletion data and proofs though, we get from the leveldb
// which was created by the bridge node.
func RunIBD(isTestnet bool, offsetfile string, ttldb string, sig chan bool) error {

	//Channel to alert the main loop to break
	stopGoing := make(chan bool, 1)

	//Channel to alert stopTxottl it's ok to exit
	done := make(chan bool, 1)

	go stopRunIBD(sig, stopGoing, done)

	//Check if the ttlfn given is a testnet file
	simutil.CheckTestnet(isTestnet)

	// open database
	o := new(opt.Options)
	o.CompactionTableSizeMultiplier = 8
	o.ReadOnly = true
	lvdb, err := leveldb.OpenFile(ttldb, o)
	if err != nil {
		panic(err)
	}
	defer lvdb.Close()

	pFile, err := os.OpenFile(simutil.PFilePath, os.O_RDONLY, 0400)
	if err != nil {
		return err
	}

	pOffsetFile, err := os.OpenFile(simutil.POffsetFilePath, os.O_RDONLY, 0400)
	if err != nil {
		return err
	}

	var currentOffsetHeight int
	//grab the last block height from currentoffsetheight
	//currentoffsetheight saves the last height from the offsetfile
	var currentOffsetHeightByte [4]byte
	currentOffsetHeightFile, err := os.Open(simutil.CurrentOffsetFilePath)
	if err != nil {
		panic(err)
	}
	currentOffsetHeightFile.Read(currentOffsetHeightByte[:])
	currentOffsetHeight = int(simutil.BtU32(currentOffsetHeightByte[:]))

	var height int

	var plustime time.Duration
	starttime := time.Now()

	totalTXOAdded := 0
	totalDels := 0

	var p utreexo.Pollard

	//	p.Minleaves = 1 << 30
	// p.Lookahead = 1000

	lookahead := int32(1000) // keep txos that last less than this many blocks

	//bool for stopping the scanner.Scan loop
	var stop bool

	// To send/receive blocks from blockreader()
	bchan := make(chan simutil.BlockToWrite, 10)

	// Reads block asynchronously from .dat files
	go simutil.BlockReader(bchan, currentOffsetHeight, height, simutil.OffsetFilePath)

	for ; height != currentOffsetHeight && stop != true; height++ {

		b := <-bchan

		err = genPollard(b.Txs, b.Height, &totalTXOAdded,
			lookahead, &totalDels, plustime, pFile, pOffsetFile, lvdb, &p)
		if err != nil {
			panic(err)
		}

		//if height%10000 == 0 {
		//	fmt.Printf("Block %d %s plus %.2f total %.2f proofnodes %d \n",
		//		height, newForest.Stats(),
		//		plustime.Seconds(), time.Now().Sub(starttime).Seconds(),
		//		totalProofNodes)
		//}

		if height%10000 == 0 {
			fmt.Printf("Block %d add %d del %d %s plus %.2f total %.2f \n",
				height, totalTXOAdded, totalDels, p.Stats(),
				plustime.Seconds(), time.Now().Sub(starttime).Seconds())
		}
		/*
			if height%100000 == 0 {
				fmt.Printf(MemStatString(fname))
			}
		*/

		//Check if stopSig is no longer false
		//stop = true makes the loop exit
		select {
		case stop = <-stopGoing:
		default:
		}
	}

	fmt.Println("Done Writing")

	done <- true

	return nil
}

//Here we write proofs for all the txs.
//All the inputs are saved as 32byte sha256 hashes.
//All the outputs are saved as LeafTXO type.
func genPollard(
	tx []*wire.MsgTx,
	height int,
	totalTXOAdded *int,
	lookahead int32,
	totalDels *int,
	plustime time.Duration,
	pFile *os.File,
	pOffsetFile *os.File,
	lvdb *leveldb.DB,
	p *utreexo.Pollard) error {

	var blockAdds []utreexo.LeafTXO
	blocktxs := []*simutil.Txotx{new(simutil.Txotx)}
	plusstart := time.Now()

	for _, tx := range tx {

		//creates all txos up to index indicated
		txhash := tx.TxHash()
		//fmt.Println(txhash.String())
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
				blocktxs[len(blocktxs)-1].DeathHeights = make([]uint32, numoutputs)
			}
		}

		// done with this txotx, make the next one and append
		blocktxs = append(blocktxs, new(simutil.Txotx))

	}
	//TODO Get rid of this. This eats up cpu
	//we started a tx but shouldn't have
	blocktxs = blocktxs[:len(blocktxs)-1]
	// call function to make all the db lookups and find deathheights
	lookupBlock(blocktxs, lvdb)

	for _, blocktx := range blocktxs {
		adds, err := genLeafTXO(blocktx, uint32(height+1))
		if err != nil {
			return err
		}
		for _, a := range adds {

			if a.Duration == 0 {
				continue
			}
			//fmt.Println("lookahead: ", lookahead)
			a.Remember = a.Duration < lookahead
			//fmt.Println("Remember", a.Remember)

			*totalTXOAdded++

			blockAdds = append(blockAdds, a)
			//fmt.Println("adds:", blockAdds)
		}
	}
	donetime := time.Now()
	plustime += donetime.Sub(plusstart)
	bpBytes, err := getProof(uint32(height), pFile, pOffsetFile)
	if err != nil {
		return err
	}
	bp, err := utreexo.FromBytesBlockProof(bpBytes)
	if err != nil {
		return err
	}
	if len(bp.Targets) > 0 {
		fmt.Printf("block proof for block %d targets: %v\n", height+1, bp.Targets)
	}
	err = p.IngestBlockProof(bp)
	if err != nil {
		return err
	}

	// totalTXOAdded += len(blockAdds)
	*totalDels += len(bp.Targets)

	err = p.Modify(blockAdds, bp.Targets)
	if err != nil {
		return err
	}
	return nil
}

// Gets the proof for a given block height
func getProof(height uint32, pFile *os.File, pOffsetFile *os.File) ([]byte, error) {

	var offset [4]byte
	pOffsetFile.Seek(int64(height*4), 0)
	pOffsetFile.Read(offset[:])

	pFile.Seek(int64(simutil.BtU32(offset[:])), 0)

	var heightbytes [4]byte
	pFile.Read(heightbytes[:])

	var compare0 [4]byte
	copy(compare0[:], heightbytes[:])

	var compare1 [4]byte
	copy(compare1[:], utreexo.U32tB(height))
	//check if height matches
	if compare0 != compare1 {
		return nil, fmt.Errorf("Corrupted proofoffset file\n")
	}

	var proofsize [4]byte
	pFile.Read(proofsize[:])

	proof := make([]byte, int(simutil.BtU32(proofsize[:])))
	pFile.Read(proof[:])

	return proof, nil

}

// plusLine reads in a line of text, generates a utxo leaf, and determines
// if this is a leaf to remember or not.
func genLeafTXO(tx *simutil.Txotx, height uint32) ([]utreexo.LeafTXO, error) {
	//fmt.Println("DeathHeights len:", len(tx.deathHeights))
	adds := []utreexo.LeafTXO{}
	for i := 0; i < len(tx.DeathHeights); i++ {
		if tx.Unspendable[i] == true {
			continue
		}
		utxostring := fmt.Sprintf("%s;%d", tx.Outputtxid, i)
		addData := utreexo.LeafTXO{
			Hash:     utreexo.HashFromString(utxostring),
			Duration: int32(tx.DeathHeights[i] - height)}
		adds = append(adds, addData)
		// fmt.Printf("expire in\t%d remember %v\n", ttlval[i], addData.Remember)
	}
	return adds, nil
}

/*
func MemStatString(fname string) string {
	var s string
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	if m.Alloc > maxmalloc {
		maxmalloc = m.Alloc

		// overwrite profile to get max mem usage
		// (only measured at 1000 block increments...)
		memfile, err := os.Create(fname)
		if err != nil {
			panic(err.Error())
		}
		pprof.WriteHeapProfile(memfile)
		memfile.Close()
	}
	// For info on each, see: https://golang.org/pkg/runtime/#MemStats
	s = fmt.Sprintf("alloc %d MB max %d MB", m.Alloc>>20, maxmalloc>>20)
	s += fmt.Sprintf("\ttotalAlloc %d MB", m.TotalAlloc>>20)
	s += fmt.Sprintf("\tsys %d MB", m.Sys>>20)
	s += fmt.Sprintf("\tnumGC %d\n", m.NumGC)
	return s
}
*/

func stopRunIBD(sig chan bool, stopGoing chan bool, done chan bool) {
	<-sig
	fmt.Println("Exiting...")

	//Tell Runibd() to finish the block it's working on
	stopGoing <- true

	//Wait Runidb() says it's ok to quit
	<-done
	os.Exit(0)
}
