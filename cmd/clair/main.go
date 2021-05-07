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
	maxHold := 10
	clearSlice := []int{600, 3000, 6000, 30000, 60000, 300000}
	numTotalRemembers, maxRemembers := LookAheadResetSlice(allCBlocks, clearSlice, maxHold)
	fmt.Println("done with look ahead")
	numTotalRemembersBehind, utxoCounter := LookBehindResetSlice(allCBlocks, clearSlice, maxRemembers)
	fmt.Println("done with look behind")
	numTotalOutputs, numRemembers, err := genClairResetSlice(allCBlocks, clearSlice, maxRemembers)
	fmt.Println("done with clairvoy")
	//maxHoldsSlice := []int{10, 200, 400, 600, 800, 1000, 1200, 1400, 1600, 1800, 2000, 20000, 20000, 160000}
	//maxHoldsSlice := []int{1,10,20}
	/*numTotalRemembers, maxRemembers := LookAheadSlice(allCBlocks, maxHoldsSlice)
	fmt.Println("done with look ahead")
	numTotalRemembersBehind,utxoCounter := LookBehindSlice(allCBlocks, maxRemembers)
	fmt.Println("done with look behind")
	numTotalOutputs, numRemembers, err := genClairSlice(allCBlocks, maxRemembers)
	fmt.Println("done with clairvoy")*/
	/*lookaheads := make([]int,len(maxHoldsSlice))
	lookbehinds := make([]int,len(maxHoldsSlice))
	clairvoys := make([]int,len(maxHoldsSlice))
	for i:= 0;i<len(maxHoldsSlice);i++{
		a,maxmem:= LookAhead(allCBlocks,maxHoldsSlice[i])
		fmt.Println("For hold of ",maxHoldsSlice[i]," lookahead is : ",a)
		b,_:= LookBehind(allCBlocks,maxmem)
		fmt.Println("For hold of ",maxHoldsSlice[i]," lookbehind is : ",b)
		_,c,_:= genClair(allCBlocks,maxmem)
		fmt.Println("For hold of ",maxHoldsSlice[i]," clairvoy is : ",c)
		lookaheads[i] = a
		lookbehinds[i] = b
		clairvoys[i] = c
	}*/
	file, err := os.Create("resetMainnet.csv")
	defer file.Close()
	writer := csv.NewWriter(file)
	defer writer.Flush()
	if err != nil {
		panic(err)
	}
	/*all := make([][]string, len(maxHoldsSlice))
	for i := 0; i < len(maxHoldsSlice); i++ {
		fmt.Println("Total outputs for hold size ",
			maxHoldsSlice[i], ": ", numTotalOutputs)
		fmt.Println("Lookahead  slice remembers :", numTotalRemembers[i])
		fmt.Println("Lookahead remembers :", lookaheads[i])
		fmt.Println("Lookbehind slice remembers :", numTotalRemembersBehind[i])
		fmt.Println("Lookbehind remembers :", lookbehinds[i])
		fmt.Println("Clairvoy slice remembers :", numRemembers[i])
		fmt.Println("Clairvoy remembers :", clairvoys[i])
		curr := make([]string, 7)
		curr[0] = fmt.Sprint(maxHoldsSlice[i])
		curr[1] = fmt.Sprint(numTotalRemembers[i])
		curr[2] = fmt.Sprint(lookaheads[i])
		curr[3] = fmt.Sprint(numTotalRemembersBehind[i])
		curr[4] = fmt.Sprint(lookbehinds[i])
		curr[5] = fmt.Sprint(numRemembers[i])
		curr[6] = fmt.Sprint(clairvoys[i])
		all[i] = curr
	}*/
	/*all := make([][]string, len(maxHoldsSlice))
	for i := 0; i < len(maxHoldsSlice); i++ {
		fmt.Println("Total outputs for hold size ",
			maxHoldsSlice[i], ": ", numTotalOutputs)
		fmt.Println("Lookahead  slice remembers :", numTotalRemembers[i])
		fmt.Println("Lookbehind slice remembers :", numTotalRemembersBehind[i])
		fmt.Println("Clairvoy slice remembers :", numRemembers[i])
		curr := make([]string, 4)
		curr[0] = fmt.Sprint(maxHoldsSlice[i])
		curr[1] = fmt.Sprint(int(numTotalOutputs) - numTotalRemembers[i])
		curr[2] = fmt.Sprint(utxoCounter - numTotalRemembersBehind[i])
		curr[3] = fmt.Sprint(int(numTotalOutputs) - numRemembers[i])
		all[i] = curr
	}
	err = writer.WriteAll(all)
	if err != nil {
		panic(err)
	}*/
	all := make([][]string, len(clearSlice))
	fmt.Println("Total outputs for hold size: ", numTotalOutputs)
	for i := 0; i < len(clearSlice); i++ {
		fmt.Println("Lookahead  remembers for clear size ", clearSlice[i], ": ", numTotalRemembers[i])
		fmt.Println("Lookbehind  remembers for clear size ", clearSlice[i], ": ", numTotalRemembersBehind[i])
		fmt.Println("Clairvoy  remembers for clear size ", clearSlice[i], ": ", numRemembers[i])
		curr := make([]string, 4)
		curr[0] = fmt.Sprint(clearSlice[i])
		curr[1] = fmt.Sprint(int(numTotalOutputs) - numTotalRemembers[i])
		curr[2] = fmt.Sprint(utxoCounter - numTotalRemembersBehind[i])
		curr[3] = fmt.Sprint(int(numTotalOutputs) - numRemembers[i])
		all[i] = curr
	}
	err = writer.WriteAll(all)
	if err != nil {
		panic(err)
	}
	file, err = os.Create("maxRemembers.csv")
	defer file.Close()
	writer = csv.NewWriter(file)
	defer writer.Flush()
	if err != nil {
		panic(err)
	}
	all = make([][]string, len(maxRemembers))
	for i := 0; i < len(maxRemembers); i++ {
		fmt.Println("Maximum  remembers for clear size ", clearSlice[i], ": ", maxRemembers[i])
		curr := make([]string, 2)
		curr[0] = fmt.Sprint(clearSlice[i])
		curr[1] = fmt.Sprint(maxRemembers[i])
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
		for j, ttl := range cblocks[i].ttls {
			if ttl == 0 {
				cblocks[i].ttls[j] = 2147483600
			}
		}
	}
	return cblocks, nil
}
