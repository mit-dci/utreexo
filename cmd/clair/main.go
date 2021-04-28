package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/mit-dci/utreexo/bridgenode"
	"github.com/mit-dci/utreexo/btcacc"
)

type txoEnd struct {
	txoIdx uint32 // which utxo (in order)
	end    int32  // when it dies (block height)
}
type txoEndSlice struct {
	txoIdx  uint32 // which utxo (in order)
	end     int32  // when it dies (block height)
	inSlice []bool // whether txoEnd is kept for corresponding maxmem
}

type cBlock struct {
	blockHeight int32
	ttls        []int32 // addHashes[i] corresponds with ttls[i]; same length
}

func main() {
	//fmt.Printf("reclair file reader")
	if len(os.Args) < 3 {
		fmt.Printf("usage: clair /path/to/proofs maxBlocks\n")
		return
	}
	maxBlock, err := strconv.Atoi(os.Args[2])
	if err != nil {
		fmt.Printf("usage: clair /path/to/proofs maxBlocks\n")
		fmt.Printf("maxBlocks needs to be a number, got %s - %s\n",
			os.Args[2], err.Error())
		return
	}
	// this initializes the configuration of files and directories to be read
	allCBlocks, err := getCBlocks(os.Args[1], 1, int32(maxBlock))
	//allCBlocks, err := getCBlocks(1, 100000)
	if err != nil {
		panic(err)
	}

	fmt.Println("Clairvoy done")

	maxHoldsSlice := []int{10, 200, 400, 600, 800, 1000, 1200, 1400, 1600, 1800, 2000, 20000, 20000, 160000}
	numTotalRemembers, maxRemembers := LookAheadSlice(allCBlocks, maxHoldsSlice)
	fmt.Println("done with look ahead")
	numTotalRemembersBehind := LookBehindSlice(allCBlocks, maxRemembers)
	fmt.Println("done with look behind")
	numTotalOutputs, numRemembers, err := genClairSlice(allCBlocks, maxRemembers)
	fmt.Println("done with clairvoy")
	file, err := os.Create("resultAllThree every 200 mainnet.csv")
	defer file.Close()
	writer := csv.NewWriter(file)
	defer writer.Flush()
	if err != nil {
		panic(err)
	}
	all := make([][]string, len(maxHoldsSlice))
	for i := 0; i < len(maxHoldsSlice); i++ {
		fmt.Println("Total outputs for hold size ", maxHoldsSlice[i], ": ", numTotalOutputs)
		fmt.Println("Lookahead  remembers :", numTotalRemembers[i])
		fmt.Println("Lookbehind  remembers :", numTotalRemembersBehind[i])
		fmt.Println("Clairvoy  remembers :", numRemembers[i])
		curr := make([]string, 4)
		curr[0] = fmt.Sprint(maxHoldsSlice[i])
		curr[1] = fmt.Sprint(int(numTotalOutputs) - numTotalRemembers[i])
		curr[2] = fmt.Sprint(int(numTotalOutputs) - numTotalRemembersBehind[i])
		curr[3] = fmt.Sprint(int(numTotalOutputs) - numRemembers[i])
		all[i] = curr
	}
	err = writer.WriteAll(all)
	if err != nil {
		panic(err)
	}
}

func getCBlocks(proofPath string, start int32, count int32) ([]cBlock, error) {
	// build cblock slice to return
	cblocks := make([]cBlock, count)
	var proofdir bridgenode.ProofDir

	//Change lines below to the path of your proof and proofoffset files on your computer
	proofdir.PFile = filepath.Join(proofPath, "proof.dat")
	proofdir.POffsetFile = filepath.Join(proofPath, "proofoffset.dat")

	// grab utreexo data and populate cblocks
	for i, _ := range cblocks {
		udataBytes, err := bridgenode.GetUDataBytesFromFile(
			proofdir, start+int32(i))
		if err != nil {
			return nil, err
		}
		udbuf := bytes.NewBuffer(udataBytes)
		var udata btcacc.UData
		udata.Deserialize(udbuf)
		// put together the cblock
		// height & ttls we can get right away in the format we need from udata
		cblocks[i].blockHeight = udata.Height
		cblocks[i].ttls = udata.TxoTTLs
	}
	return cblocks, nil
}
