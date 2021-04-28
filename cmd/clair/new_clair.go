package main

/*

What proportion is remembered vs if we just remember ttls with less than 10. Can do a similar method
Scp -r proofdata
Scp copies ssh

*/
import (
	"fmt"
	"path/filepath"
	"strconv"

	//"io/ioutil"
	"encoding/csv"
	"os"
	"sort"

	//"strconv"
	//"time"
	"bytes"

	//"github.com/mit-dci/utreexo/cmd/ibdsim"

	"github.com/mit-dci/utreexo/bridgenode"
	"github.com/mit-dci/utreexo/btcacc"
	//"github.com/mit-dci/utreexo/utreexo"
)

/* idea here:
input: load a txo / ttl file, and a memory size
output: write a bitmap of which txos to remember

how to do this:
load everything into a sorted slice (sorted by end time)
every block, remove the beginning of the slice (stuff that has died)
	- flag these as memorable; they made it to the end
add (interspersed) the new txos in the block
chop off the end of the slice (all that exceeds memory capacity)
that's all.

format of the schedule.clr file: bitmaps of 8 txos per byte.  1s mean remember, 0s mean
forget.  Not padded or anything.

format of index file: 4 bytes per block.  *Txo* position of block start, in unsigned
big endian.

So to get from a block height to a txo position, seek to 4*height in the index,
read 4 bytes, then seek to *that* /8 in the schedule file, and shift around as needed.

*/

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
	/*var maxTTL int32
	maxTTL = 0
	for i:=0;i<len(allCBlocks);i++{
		for j:=0;j<len(allCBlocks[i].ttls);j++{
			if(allCBlocks[i].ttls[j] > maxTTL){
				maxTTL = allCBlocks[i].ttls[j]
			}
		}
	}
	fmt.Println(maxTTL)*/
	//runs using clairvoyint algo
	//remembers 437/555 = 78.74%
	//numTotalOutputs, numRemembers,err := genClair(allCBlocks,287000)
	/*numTotalOutputs1, numRemembers1,err := genClair(allCBlocks,30001)
	numTotalOutputs1, numRemembers100,err := genClair(allCBlocks,921832)
	numTotalOutputs1, numRemembers1000,err := genClair(allCBlocks,1903977)*/

	fmt.Println("Clairvoy done")
	//runs using old remembering system
	//remembers 296/555 = 53.33%
	//numTotalRemembers, maxRemembers :=oldRun(1780701,1750000)

	//numTotalRemembers, maxRemembers, currSumSlice :=LookAhead(allCBlocks,10)

	//maxHoldsSlice := []int{1,10,100,1000}
	//maxHoldsSlice := []int{10,200,400,600,800,1000,1200,1400,1600,1800,2000}
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
	/*for _, value := range all {
	        err := writer.Write(value)
	        if(err != nil){
				panic(err)
			}
		}*/
	/*maxHoldsSlice := []int{30001,287000}
	numTotalRemembersBehind, maxRemembersBehind :=LookBehindSlice(allCBlocks,maxHoldsSlice)*/

	//numTotalRemembersBehind, maxRemembersBehind :=LookBehind(allCBlocks,287000)

	//numTotalRemembersBehind1, maxRemembersBehind1 :=LookBehind(allCBlocks,30001)
	//numTotalRemembersBehind100, maxRemembersBehind100 :=LookBehind(allCBlocks,921832)
	//numTotalRemembersBehind1000, maxRemembersBehind1000 :=LookBehind(allCBlocks,1903977)

	/*for i := 0; i < len(maxHoldsSlice); i++ {
		fmt.Println("total number of remembers for look ahead ", maxHoldsSlice[i],": ",numTotalRemembers[i])
		fmt.Println("max number of remembers for look ahead: ", maxHoldsSlice[i],": ",maxRemembers[i])
		file, err := os.Create(fmt.Sprintf("result%d.csv",maxHoldsSlice[i]))
		writer := csv.NewWriter(file)
		if(err!= nil){
			panic(err)
		}
		for _, value := range currSumSlice[i] {
			err := writer.Write(value)
			if(err != nil){
				panic(err)
			}
		}
	}*/

	/*for i := 0; i < len(maxHoldsSlice); i++ {
		fmt.Println("total number of remembers for mem size ", maxHoldsSlice[i],": ",numTotalRemembersBehind[i])
		fmt.Println("max number of remembers for mem size: ", maxHoldsSlice[i],": ",maxRemembersBehind[i])
	}*/
	/*file, err := os.Create("resultAll.csv")
		writer := csv.NewWriter(file)
		if(err!= nil){
			panic(err)
		}
		for _, value := range currSumSlice {
	        err := writer.Write(value)
	        if(err != nil){
				panic(err)
			}
		}*/

	//fmt.Println("total number of remembers for look ahead : ",numTotalRemembers)
	//fmt.Println("max number of remembers for look ahead: : ",maxRemembers)

	/*fmt.Println("total number of remembers for CLAIRVOY 1:",numRemembers1)
	fmt.Println("total number of remembers for CLAIRVOY 100:",numRemembers100)
	fmt.Println("total number of remembers for CLAIRVOY 1000:",numRemembers1000)*/

	//fmt.Println("total number of remembers for CLAIRVOY 10:",numRemembers)
	//fmt.Println("all Blocks: ",numTotalOutputs)
	/*fmt.Println("total number of remembers for look behind 1: ",numTotalRemembersBehind1)
	fmt.Println("max number of remembers for look behind 1: ",maxRemembersBehind1)
	fmt.Println("total number of remembers for look behind 100: ",numTotalRemembersBehind100)
	fmt.Println("max number of remembers for look behind 100: ",maxRemembersBehind100)
	fmt.Println("total number of remembers for look behind 1000: ",numTotalRemembersBehind1000)
	fmt.Println("max number of remembers for look behind 1000: ",maxRemembersBehind1000)*/
	//fmt.Println("total number of remembers for look behind 1:0 ",numTotalRemembersBehind)
	//fmt.Println("max number of remembers for look behind 10: ",maxRemembersBehind)
}

/*Run utreexoserver exe file on the tn3 blocks folder*/
/*build utreexoserver */
/* blocks are at ut/testnet3/blocks
/* ./utreexoserver -net=testnet -forest=disk -datadir=. -bridgedir=.*/
/* navigate to
/*ctrl+b n switch*.
/*ctrl+b c new one*/
/* ctrl+b d*/
/* tmux a*/
/* scp utreexoserver 34.105.121.136:~/ut for the new_clair.go not utreexoserver*/
/*scp new_clair.go 34.105.121.136:~/go/src/github.com/mit-dci/utreexo/cmd/clair*/
/*scp 34.105.121.136:~/go/src/github.com/mit-dci/utreexo/cmd/clair/result.csv /Users/cb/Desktop/MIT/UROP/Spring\ 2021*/
/*GOOS=linux go build -v  for */

// NOTE I think we don't actually need to keep track of insertions or deletions
// at all, and ONLY the TTLs are needed!
// Because, who cares *what* the element being added is, the only reason to
// be able to identify it is so we can find it to remove it.  But we
// can assign it a sequential number instead of using a hash

func getCBlocks(proofPath string, start int32, count int32) ([]cBlock, error) {
	// build cblock slice to return
	cblocks := make([]cBlock, count)
	/*print("getting blocks\n")
	print(len(cblocks))
	print("\n")*/
	var proofdir bridgenode.ProofDir

	//Change lines below to the path of your proof and proofoffset files on your computer
	proofdir.PFile = filepath.Join(proofPath, "proof.dat")
	proofdir.POffsetFile = filepath.Join(proofPath, "proofoffset.dat")

	/*proofdir.PFile = "/home/cb/ut/mainnet/proofdata/proof.dat"
	proofdir.POffsetFile = "/home/cb/ut/mainnet/proofdata/offset.dat"*/

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

type sortableTxoSlice []txoEnd

func (s sortableTxoSlice) Len() int      { return len(s) }
func (s sortableTxoSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s sortableTxoSlice) Less(i, j int) bool {
	return s[i].end < s[j].end
}

func (s *sortableTxoSlice) MergeSort(a sortableTxoSlice) {
	*s = append(*s, a...)
	sort.Sort(s)
}

// assumes a sorted slice.  Splits on a "end" value, returns the low slice and
// leaves the higher "end" value sequence in place
func SplitAfter(s sortableTxoSlice, h int32) (top, bottom sortableTxoSlice) {
	for i, c := range s {
		if c.end > h {
			top = s[0:i]   // return the beginning of the slice
			bottom = s[i:] // chop that part off
			break
		}
	}
	if top == nil {
		bottom = s
	}
	return
}

// basically flips bit n of a big file to 1.
func assertBitInFile(txoIdx uint32, scheduleFile *os.File) error {
	offset := int64(txoIdx / 8)
	b := make([]byte, 1)
	_, err := scheduleFile.ReadAt(b, offset)
	if err != nil {
		return err
	}
	b[0] = b[0] | 1<<(7-(txoIdx%8))
	_, err = scheduleFile.WriteAt(b, offset)
	return err
}

// flips a bit to 1.  Crashes if you're out of range.
func assertBitInRam(txoIdx uint32, scheduleSlice []byte) {
	offset := int64(txoIdx / 8)
	scheduleSlice[offset] |= 1 << (7 - (txoIdx % 8))
}
func LookBehindResetSlice(allCBlocks []cBlock, resetSizes []int, maxmems []int) ([]int) {
    cache := make([][]int, len(resetSizes))
    deletion := make([][][]int,len(resetSizes))
    for i := 0; i < len(resetSizes); i++ {
        cache[i] = make([]int,0)
        deletion[i] = make([][]int, resetSizes[i])
        for j:=0;j<resetSizes[i];j++{
            deletion[i][j] =  make([]int,0)
        }
    }
    memPointers := make([]int, len(maxmems))
    utxoCounter := 0
    totalRemembers := make([]int, len(maxmems))
    for i := 0; i < len(allCBlocks); i++ {
        for j := 0; j < len(resetSizes); j++ {
            if(i%resetSizes[j] == 0){
                cache[j] = cache[j][:0]
                deletion[j] = make([][]int,resetSizes[j])
                for k:=0;k<resetSizes[j];k++{
                    deletion[j][k] =  make([]int,0)
                }
            }
        }
        for j := 0; j < len(allCBlocks[i].ttls); j++ {
            //if lives too long and we don't look at that block to delete, then just ignore
            for k:=0;k<len(resetSizes);k++{
                if(allCBlocks[i].ttls[j] >= int32(len(deletion[k]))){
                    continue
                }
                deletion[k][allCBlocks[i].ttls[j]] = append(deletion[k][allCBlocks[i].ttls[j]],utxoCounter)
                cache[k] = append(cache[k], utxoCounter)
            }
            utxoCounter += 1
        }
        numRemembers := make([]int, len(resetSizes))
        // The way cache and deletion are built, both should always be sorted 
        for j:=0;j<len(resetSizes);j++{
            currDelPos := len(deletion[j][0])-1
            currCachePos := len(cache[j])-1
            for (currDelPos >= 0 && currCachePos >=0){
                for (currDelPos >= 0 && deletion[j][0][currDelPos] > cache[j][currCachePos]){
                    //continue incrementing deletion pos if cache already passed it
                    currDelPos -= 1
                }
                if(currDelPos < 0){
                    break
                }
                if(deletion[j][0][currDelPos] == cache[j][currCachePos]){
                    // we found it! This means we remembered it and we can increment 
                    if(memPointers[j] <= currCachePos){
                        // this is remembered for this specific size
                        numRemembers[j] += 1
                        totalRemembers[j] += 1
                    }
                }
                currDelPos -= 1 
                //remove from cache
                cache[j] = append(cache[j][:currCachePos],cache[j][currCachePos+1:]...)
                currCachePos -= 1   
            }
            deletion[j] = deletion[j][1:]
            trimPos := len(cache[j]) - maxmems[j]
            if(trimPos > 0){
                cache[j] = cache[j][trimPos:]
            }
        }
    }
    return totalRemembers
}

func LookBehindSlice(allCBlocks []cBlock, maxmems []int) []int {
	cache := make([]int, 0)
	deletion := make([][]int, len(allCBlocks))
	for i := 0; i < len(allCBlocks); i++ {
		deletion[i] = make([]int, 0)
	}
	memPointers := make([]int, len(maxmems))
	utxoCounter := 0
	totalRemembers := make([]int, len(maxmems))
	maxRemembers := make([]int, len(maxmems))
	for i := 0; i < len(allCBlocks); i++ {
		oldLenCache := len(cache)
		for j := 0; j < len(allCBlocks[i].ttls); j++ {
			//if lives too long and we don't look at that block to delete, then just ignore
			if allCBlocks[i].ttls[j] >= int32(len(deletion)) {
				continue
			}
			deletion[allCBlocks[i].ttls[j]] = append(deletion[allCBlocks[i].ttls[j]], utxoCounter)
			cache = append(cache, utxoCounter)
			utxoCounter += 1
		}
		// The way cache and deletion are built, both should always be sorted
		currDelPos := len(deletion[0]) - 1
		currCachePos := len(cache) - 1
		numRemembers := make([]int, len(maxmems))
		for currDelPos >= 0 && currCachePos >= 0 {
			for currDelPos >= 0 && deletion[0][currDelPos] > cache[currCachePos] {
				//continue incrementing deletion pos if cache already passed it
				currDelPos -= 1
			}
			if currDelPos < 0 {
				break
			}
			if deletion[0][currDelPos] == cache[currCachePos] {
				// we found it! This means we remembered it and we can increment
				for j := 0; j < len(maxmems); j++ {
					if memPointers[j] <= currCachePos {
						// this is remembered for this specific size
						numRemembers[j] += 1
						totalRemembers[j] += 1
					}
				}
				currDelPos -= 1
				//remove from cache
				cache = append(cache[:currCachePos], cache[currCachePos+1:]...)
			}
			currCachePos -= 1

		}
		deletion = deletion[1:]

		/* UPDATE CACHE ACCORDINGLY */
		for j := 0; j < len(maxmems); j++ {
			/*trimPos := len(cache) - maxmems[j]
			if(trimPos > 0){
				memPointers[j] = trimPos
			}
			if(len(cache)-trimPos > maxRemembers[j]){
				maxRemembers[j] = len(cache)-trimPos
			}*/
			lenOfNewCache := oldLenCache - memPointers[j] + len(allCBlocks[i].ttls) - numRemembers[j]
			if lenOfNewCache >= maxmems[j] {
				memPointers[j] = len(cache) - maxmems[j]
				maxRemembers[j] = maxmems[j]
			} else {
				memPointers[j] = len(cache) - lenOfNewCache
				if lenOfNewCache > maxRemembers[j] {
					maxRemembers[j] = lenOfNewCache
				}
			}
		}
	}
	return totalRemembers
}

func LookBehind(allCBlocks []cBlock, maxmem int) (int, int) {
	cache := make([]int, 0)
	deletion := make([][]int, len(allCBlocks))
	for i := 0; i < len(allCBlocks); i++ {
		deletion[i] = make([]int, 0)
	}
	utxoCounter := 0
	totalRemembers := 0
	maxRemembers := 0
	for i := 0; i < len(allCBlocks); i++ {
		for j := 0; j < len(allCBlocks[i].ttls); j++ {
			//if lives too long and we don't look at that block to delete, then just ignore
			if allCBlocks[i].ttls[j] >= int32(len(deletion)) {
				continue
			}
			deletion[allCBlocks[i].ttls[j]] = append(deletion[allCBlocks[i].ttls[j]], utxoCounter)
			cache = append(cache, utxoCounter)
			utxoCounter += 1
		}
		// The way cache and deletion are built, both should always be sorted
		currDelPos := 0
		currCachePos := 0
		numRemember := 0
		for currDelPos < len(deletion[0]) && currCachePos < len(cache) {
			for currDelPos < len(deletion[0]) && deletion[0][currDelPos] < cache[currCachePos] {
				//continue incrementing deletion pos if cache already passed it
				currDelPos += 1
			}
			if currDelPos >= len(deletion[0]) {
				break
			}
			if deletion[0][currDelPos] == cache[currCachePos] {
				// we found it! This means we remembered it and we can increment
				numRemember += 1
				currDelPos += 1
				//remove from cache
				cache = append(cache[:currCachePos], cache[currCachePos+1:]...)
			} else {
				currCachePos += 1
			}

		}
		totalRemembers += numRemember
		deletion = deletion[1:]

		/* UPDATE CACHE ACCORDINGLY */
		trimPos := len(cache) - maxmem
		if trimPos > 0 {
			cache = cache[trimPos:]
		}

		if len(cache) > maxRemembers {
			maxRemembers = len(cache)
		}
	}
	return totalRemembers, maxRemembers
}

func LookAheadResetSlice(allCBlocks []cBlock, maxResets []int, maxHold int) ([]int,[]int,) {
    currRemembers := make([][]int, len(maxResets))
    for i := 0; i < len(maxResets); i++ {
        currRemembers[i] = make([]int, maxHold)
    }
    totalRemembers := make([]int, len(maxResets))
    maxRemembers := make([]int, len(maxResets))
    prevSum := make([]int, len(maxResets))
    currSum := make([]int, len(maxResets))
    //currSumStores := make([][][]string, len(maxHolds))
    //writers := make([]Writer,len(maxHolds))
    /*for i := 0; i < len(maxHolds); i++ {
        currSumStores[i] = make([][]string,len(allCBlocks))
        for j := 0; j < len(allCBlocks); j++ {
            currSumStores[i][j] = make([]string,2)
        }
    }*/
    for i := 0; i < len(allCBlocks); i++ {
        /*currBlocks, err := getCBlocks(int32(i)+1,1)
        currBlock := currBlocks[0]
        if(err != nil){
            panic(err)
        }*/
        if(i%100 == 0){
            fmt.Println("On block: ",i)
        }
        for j := 0; j < len(maxResets); j++ {
            if(i%maxResets[j] == 0){
                prevSum[j] = 0
                currSum[j] = 0
                currRemembers[j] = make([]int,maxHold)
            }
        }
        numRemember := make([]int,len(maxResets))
        for j := 0; j < len(allCBlocks[i].ttls); j++ {
            for k:= 0;k <len(maxResets);k++{
                if(allCBlocks[i].ttls[j] <= int32(maxHold) && int32(maxResets[k]-i) >= allCBlocks[i].ttls[j]){
                    numRemember[k] += 1
                }
            }
        }
        for j := 0; j < len(maxResets); j++ {
            if (i<maxHold){
                currRemembers[j][i] = numRemember[j]
                currSum[j] = prevSum[j] + numRemember[j]
                prevSum[j] = currSum[j]
            }else{
                currRemembers[j] = append(currRemembers[j], numRemember[j])
                currSum[j] = prevSum[j] + numRemember[j] - currRemembers[j][0]
                currRemembers[j] = currRemembers[j][1:]
                prevSum[j] = currSum[j]
            }
            //currSumStores[j][i][0] = fmt.Sprint(i)
            //currSumStores[j][i][1] = fmt.Sprint(currSum[j])
            if(currSum[j] > maxRemembers[j]){
                maxRemembers[j] = currSum[j]
            }
            totalRemembers[j] += numRemember[j]
        }
    }
    //fmt.Println("total number of remembers for gen10: ",totalRemembers)
    //fmt.Println("max number of remembers for gen10: ",maxRemembers)
    return totalRemembers, maxRemembers
}


func LookAheadSlice(allCBlocks []cBlock, maxHolds []int) ([]int, []int) {
	currRemembers := make([][]int, len(maxHolds))
	for i := 0; i < len(maxHolds); i++ {
		currRemembers[i] = make([]int, maxHolds[i])
	}
	totalRemembers := make([]int, len(maxHolds))
	maxRemembers := make([]int, len(maxHolds))
	prevSum := make([]int, len(maxHolds))
	currSum := make([]int, len(maxHolds))
	//currSumStores := make([][][]string, len(maxHolds))
	//writers := make([]Writer,len(maxHolds))
	/*for i := 0; i < len(maxHolds); i++ {
		currSumStores[i] = make([][]string,len(allCBlocks))
		for j := 0; j < len(allCBlocks); j++ {
			currSumStores[i][j] = make([]string,2)
		}
	}*/
	for i := 0; i < len(allCBlocks); i++ {
		/*currBlocks, err := getCBlocks(int32(i)+1,1)
		currBlock := currBlocks[0]
		if(err != nil){
			panic(err)
		}*/
		if i%100 == 0 {
			fmt.Println("On block: ", i)
		}
		numRemember := make([]int, len(maxHolds))
		for j := 0; j < len(allCBlocks[i].ttls); j++ {
			for k := 0; k < len(maxHolds); k++ {
				if allCBlocks[i].ttls[j] <= int32(maxHolds[k]) {
					numRemember[k] += 1
				}
			}
		}
		for j := 0; j < len(maxHolds); j++ {
			if i < maxHolds[j] {
				currRemembers[j][i] = numRemember[j]
				currSum[j] = prevSum[j] + numRemember[j]
				prevSum[j] = currSum[j]
			} else {
				currRemembers[j] = append(currRemembers[j], numRemember[j])
				currSum[j] = prevSum[j] + numRemember[j] - currRemembers[j][0]
				currRemembers[j] = currRemembers[j][1:]
				prevSum[j] = currSum[j]
			}
			//currSumStores[j][i][0] = fmt.Sprint(i)
			//currSumStores[j][i][1] = fmt.Sprint(currSum[j])
			if currSum[j] > maxRemembers[j] {
				maxRemembers[j] = currSum[j]
			}
			totalRemembers[j] += numRemember[j]
		}
	}
	//fmt.Println("total number of remembers for gen10: ",totalRemembers)
	//fmt.Println("max number of remembers for gen10: ",maxRemembers)
	return totalRemembers, maxRemembers
}

func LookAhead(allCBlocks []cBlock, maxHold int) (int, int, [][]string) {
	currRemembers := make([]int, maxHold)
	totalRemembers := 0
	maxRemembers := 0
	prevSum := 0
	currSumStores := make([][]string, len(allCBlocks))
	for i := 0; i < len(allCBlocks); i++ {
		currSumStores[i] = make([]string, 2)
		currSumStores[i][0] = fmt.Sprint(i)
		/*currBlocks, err := getCBlocks(int32(i)+1,1)
		currBlock := currBlocks[0]
		if(err != nil){
			panic(err)
		}*/
		if i%100 == 0 {
			fmt.Println("On block: ", i)
		}
		numRemember := 0
		for j := 0; j < len(allCBlocks[i].ttls); j++ {
			if allCBlocks[i].ttls[j] <= int32(maxHold) {
				numRemember += 1
			}
		}
		var currSum int
		if i < maxHold {
			currRemembers[i] = numRemember
			currSum = prevSum + numRemember
			prevSum = currSum
		} else {
			currRemembers = append(currRemembers, numRemember)
			currSum = prevSum + numRemember - currRemembers[0]
			currRemembers = currRemembers[1:]
			prevSum = currSum
		}
		currSumStores[i][1] = fmt.Sprint(currSum)
		if currSum > maxRemembers {
			maxRemembers = currSum
		}
		totalRemembers += numRemember
	}
	fmt.Println("total number of remembers for gen10: ", totalRemembers)
	fmt.Println("max number of remembers for gen10: ", maxRemembers)
	return totalRemembers, maxRemembers, currSumStores
}

func genClairSlice(allCBlocks []cBlock, maxmems []int) (uint32, []int, error) {
	//scheduleSlice := make([]byte, 125000000)
	clairSlices := make([]sortableTxoSlice, len(maxmems))
	var utxoCounter uint32
	utxoCounter = 0
	var allCounts uint32
	allCounts = 0
	numRemembers := make([]int, len(maxmems))
	for i := 0; i < len(allCBlocks); i++ {
		/*currBlocks,err := getCBlocks(int32(i)+1,1)
		if(err != nil){
			panic(err)
		}
		currBlock := currBlocks[0]*/
		var blockEnds sortableTxoSlice
		if i%100 == 0 {
			fmt.Println("On block: ", i)
		}

		//another for loop going through ttls. utxocounter increment for ttls not blocks
		for j := 0; j < len(allCBlocks[i].ttls); j++ {
			allCounts += 1
			var e txoEnd
			e = txoEnd{
				txoIdx: utxoCounter,
				end:    allCBlocks[i].blockHeight + allCBlocks[i].ttls[j],
			}
			utxoCounter++
			blockEnds = append(blockEnds, e)
		}
		sort.SliceStable(blockEnds, func(i, j int) bool {
			return blockEnds[i].end < blockEnds[j].end
		})
		for j := 0; j < len(maxmems); j++ {
			clairSlices[j] = mergeSortedSlices(clairSlices[j], blockEnds)
			var remembers sortableTxoSlice
			remembers, clairSlices[j] = SplitAfter(clairSlices[j], allCBlocks[i].blockHeight)

			numRemembers[j] += len(remembers)
			if len(clairSlices[j]) > maxmems[j] {
				clairSlices[j] = clairSlices[j][:maxmems[j]]
			}
		}
		//add counter that cumulatively counts how many we are remembering(i.e. density of schedule)
		/*if len(remembers) > 0 {
			for _, r := range remembers {
				assertBitInRam(r.txoIdx, scheduleSlice)
			}
		}*/
	}
	//fileString := fmt.Sprintf("schedule%dpos.clr", maxmem)
	/* How should I write this part?*/
	//ioutil.WriteFile(fileString, scheduleSlice, 0644)
	//scheduleSlice = nil
	fmt.Println("total number of remembers for CLAIRVOY:", numRemembers)
	fmt.Println("all Blocks: ", allCounts)
	return allCounts, numRemembers, nil
}
func genClairResetSlice(allCBlocks []cBlock, resetSize []int,maxmems []int) (uint32, []int, error) {
    //scheduleSlice := make([]byte, 125000000)
    resetSlices := make([]sortableTxoSlice,len(resetSize))
    var utxoCounter uint32
    utxoCounter = 0
    var allCounts uint32
    allCounts = 0
    numRemembers := make([]int,len(resetSize))
    for i := 0; i < len(allCBlocks); i++ {
        /*currBlocks,err := getCBlocks(int32(i)+1,1)
        if(err != nil){
            panic(err)
        }
        currBlock := currBlocks[0]*/
        for j := 0; j < len(resetSize); j++ {
            if (i%resetSize[j] == 0){
                resetSlices[j] = resetSlices[j][:0]
            }
        }
        var blockEnds sortableTxoSlice
        if(i%100 == 0){
            fmt.Println("On block: ",i)
        }
        
        //another for loop going through ttls. utxocounter increment for ttls not blocks
        for j := 0; j < len(allCBlocks[i].ttls); j++ {
            allCounts += 1
            var e txoEnd
            e = txoEnd{
                txoIdx: utxoCounter,
                end:    allCBlocks[i].blockHeight + allCBlocks[i].ttls[j],
            }
            utxoCounter++
            blockEnds = append(blockEnds, e)
        }
        sort.SliceStable(blockEnds, func(i, j int) bool {
            return blockEnds[i].end < blockEnds[j].end
        })
        for j := 0; j < len(maxmems); j++ {
            resetSlices[j] = mergeSortedSlices(resetSlices[j], blockEnds)
            var remembers sortableTxoSlice
            remembers, resetSlices[j] = SplitAfter(resetSlices[j], allCBlocks[i].blockHeight)
            numRemembers[j] += len(remembers)
            if len(resetSlices[j]) > maxmems[j] {
                resetSlices[j] = resetSlices[j][:maxmems[j]]
            }
        }
        //add counter that cumulatively counts how many we are remembering(i.e. density of schedule)
        /*if len(remembers) > 0 {
            for _, r := range remembers {
                assertBitInRam(r.txoIdx, scheduleSlice)
            }
        }*/
    }
    //fileString := fmt.Sprintf("schedule%dpos.clr", maxmem)
    /* How should I write this part?*/
    //ioutil.WriteFile(fileString, scheduleSlice, 0644)
    //scheduleSlice = nil
    fmt.Println("total number of remembers for CLAIRVOY:",numRemembers)
    fmt.Println("all Blocks: ",allCounts)
    return allCounts, numRemembers, nil
}


func genClair(allCBlocks []cBlock, maxmem int) (uint32, int, error) {
	//scheduleSlice := make([]byte, 125000000)
	var clairSlice sortableTxoSlice
	var utxoCounter uint32
	utxoCounter = 0
	var allCounts uint32
	allCounts = 0
	numRemembers := 0
	for i := 0; i < len(allCBlocks); i++ {
		/*currBlocks,err := getCBlocks(int32(i)+1,1)
		if(err != nil){
			panic(err)
		}
		currBlock := currBlocks[0]*/
		var blockEnds sortableTxoSlice
		if i%100 == 0 {
			fmt.Println("On block: ", i)
		}

		//another for loop going through ttls. utxocounter increment for ttls not blocks
		for j := 0; j < len(allCBlocks[i].ttls); j++ {
			allCounts += 1
			var e txoEnd
			e = txoEnd{
				txoIdx: utxoCounter,
				end:    allCBlocks[i].blockHeight + allCBlocks[i].ttls[j],
			}
			utxoCounter++
			blockEnds = append(blockEnds, e)
		}
		sort.SliceStable(blockEnds, func(i, j int) bool {
			return blockEnds[i].end < blockEnds[j].end
		})
		clairSlice = mergeSortedSlices(clairSlice, blockEnds)

		var remembers sortableTxoSlice
		remembers, clairSlice = SplitAfter(clairSlice, allCBlocks[i].blockHeight)

		numRemembers += len(remembers)
		if len(clairSlice) > maxmem {
			clairSlice = clairSlice[:maxmem]
		}
		//add counter that cumulatively counts how many we are remembering(i.e. density of schedule)
		/*if len(remembers) > 0 {
			for _, r := range remembers {
				assertBitInRam(r.txoIdx, scheduleSlice)
			}
		}*/
	}
	//fileString := fmt.Sprintf("schedule%dpos.clr", maxmem)
	/* How should I write this part?*/
	//ioutil.WriteFile(fileString, scheduleSlice, 0644)
	//scheduleSlice = nil
	fmt.Println("total number of remembers for CLAIRVOY:", numRemembers)
	fmt.Println("all Blocks: ", allCounts)
	return allCounts, numRemembers, nil
}

// This is copied from utreexo utils, and in this cases there will be no
// duplicates, so that part is removed.  Uses sortableTxoSlices.

// mergeSortedSlices takes two slices (of uint64s; though this seems
// genericizable in that it's just < and > operators) and merges them into
// a single sorted slice, discarding duplicates.
// (eg [1, 5, 8, 9], [2, 3, 4, 5, 6] -> [1, 2, 3, 4, 5, 6, 8, 9]
func mergeSortedSlices(a sortableTxoSlice, b sortableTxoSlice) (c sortableTxoSlice) {
	maxa := len(a)
	maxb := len(b)

	// make it the right size (no dupes)
	c = make(sortableTxoSlice, maxa+maxb)

	idxa, idxb := 0, 0
	for j := 0; j < len(c); j++ {
		// if we're out of a or b, just use the remainder of the other one
		if idxa >= maxa {
			// a is done, copy remainder of b
			j += copy(c[j:], b[idxb:])
			c = c[:j] // truncate empty section of c
			break
		}
		if idxb >= maxb {
			// b is done, copy remainder of a
			j += copy(c[j:], a[idxa:])
			c = c[:j] // truncate empty section of c
			break
		}

		obja, objb := a[idxa], b[idxb]
		if obja.end < objb.end { // a is less so append that
			c[j] = obja
			idxa++
		} else { // b is less so append that
			c[j] = objb
			idxb++
		}
	}
	return
}
func mergeSortedSliceSlices(a []txoEndSlice, b []txoEndSlice) (c []txoEndSlice) {
	maxa := len(a)
	maxb := len(b)

	// make it the right size (no dupes)
	c = make([]txoEndSlice, maxa+maxb)

	idxa, idxb := 0, 0
	for j := 0; j < len(c); j++ {
		// if we're out of a or b, just use the remainder of the other one
		if idxa >= maxa {
			// a is done, copy remainder of b
			j += copy(c[j:], b[idxb:])
			c = c[:j] // truncate empty section of c
			break
		}
		if idxb >= maxb {
			// b is done, copy remainder of a
			j += copy(c[j:], a[idxa:])
			c = c[:j] // truncate empty section of c
			break
		}

		obja, objb := a[idxa], b[idxb]
		if obja.end < objb.end { // a is less so append that
			c[j] = obja
			idxa++
		} else { // b is less so append that
			c[j] = objb
			idxb++
		}
	}
	return
}
