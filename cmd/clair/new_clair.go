package main
/*

What proportion is remembered vs if we just remember ttls with less than 10. Can do a similar method
Scp -r proofdata
Scp copies ssh

*/
import (
	"fmt"
	"io/ioutil"
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
	end    int32 // when it dies (block height)
}

type cBlock struct {
	blockHeight int32
	ttls        []int32 // addHashes[i] corresponds with ttls[i]; same length
}


func main() {
	fmt.Printf("reclair file reader")

	// this initializes the configuration of files and directories to be read

	//runs using clairvoyint algo
	//remembers 437/555 = 78.74%
	Clairvoy(25000000)


	//runs using old remembering system
	//remembers 296/555 = 53.33%
	oldRun(25000000)
}
// NOTE I think we don't actually need to keep track of insertions or deletions
// at all, and ONLY the TTLs are needed!
// Because, who cares *what* the element being added is, the only reason to
// be able to identify it is so we can find it to remove it.  But we
// can assign it a sequential number instead of using a hash

func getCBlocks(start int32, count int32, cfg bridgenode.Config) ([]cBlock, error) {
	// build cblock slice to return
	cblocks := make([]cBlock, count)
	print("getting blocks\n")
	print(len(cblocks))
	print("\n")
	var proofdir bridgenode.ProofDir

	//Change lines below to the path of your proof and proofoffset files on your computer
	proofdir.PFile = "/Users/cb/Desktop/MIT/UROP/Spring 2021/proofdata/proof.dat"
	proofdir.POffsetFile = "/Users/cb/Desktop/MIT/UROP/Spring 2021/proofdata/proofoffset.dat"
	
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

func Clairvoy(mem int) error {
	fmt.Printf("genclair - builds clairvoyant caching schedule\n")

	//Channel to alert the main loop to break
	//stopGoing := make(chan bool, 1)

	//Channel to alert stopTxottl it's ok to exit
	done := make(chan bool, 1)

	scheduleSlice := make([]byte, 125000000)

	maxmem := mem
	if maxmem == 0 {
		return fmt.Errorf("usage: clair memorysize  (eg ./clair 3000)\n")

	}

	//var utxoCounter uint32
	var clairSlice sortableTxoSlice
	var remembers sortableTxoSlice
	var numTotalOutputs uint32
	cfg, err := bridgenode.Parse(os.Args[1:])
	
	if err != nil {
		panic(err)
	}
	allCBlocks,err := getCBlocks(1, 200, *cfg)	
	//print(len(allCBlocks))
	//print("\n")
	numTotalOutputs, scheduleSlice, clairSlice, remembers, err = genClair(allCBlocks, scheduleSlice, clairSlice, maxmem)
	if err != nil {
		panic(err)
	}
		
	fmt.Printf("done\n")

	fileString := fmt.Sprintf("schedule%dpos.clr", maxmem)
	/* How should I write this part?*/
	ioutil.WriteFile(fileString, scheduleSlice, 0644)
	print("total number of remembers for CLAIRVOY: ")
	print(len(remembers))
	print("\n")
	print("all Blocks: ")
	print((numTotalOutputs))
	print("\n")
	done <- true
	return nil
}

func oldRun(mem int) error {
	fmt.Printf("genclair - builds clairvoyant caching schedule\n")

	//Channel to alert the main loop to break
	//stopGoing := make(chan bool, 1)

	//Channel to alert stopTxottl it's ok to exit
	done := make(chan bool, 1)

	scheduleSlice := make([]byte, 125000000)

	maxmem := mem
	if maxmem == 0 {
		return fmt.Errorf("usage: clair memorysize  (eg ./clair 3000)\n")

	}

	//var utxoCounter uint32
	var clairSlice sortableTxoSlice
	var remembers sortableTxoSlice
	var numTotalOutputs uint32
	cfg, err := bridgenode.Parse(os.Args[1:])
	
	if err != nil {
		panic(err)
	}
	allCBlocks,err := getCBlocks(1, 200, *cfg)	
	//print(len(allCBlocks))
	//print("\n")
	numTotalOutputs, scheduleSlice, clairSlice, remembers, err = gen10(allCBlocks, scheduleSlice, clairSlice, maxmem)
	if err != nil {
		panic(err)
	}
		
	fmt.Printf("done\n")

	fileString := fmt.Sprintf("schedule%dpos.clr", maxmem)
	/* How should I write this part?*/
	ioutil.WriteFile(fileString, scheduleSlice, 0644)
	print("total number of remembers for gen10: ")
	print(len(remembers))
	print("\n")
	print("all Blocks: ")
	print((numTotalOutputs))
	print("\n")
	done <- true
	return nil
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
func gen10(cBlocks []cBlock, scheduleSlice []byte, clairSlice sortableTxoSlice, maxmem int) (uint32,[]byte, sortableTxoSlice, sortableTxoSlice, error){
	
	var utxoCounter uint32
	utxoCounter = 0
	var allCounts uint32
	allCounts = 0
	var allRemembers sortableTxoSlice
	//get rid of for loop. Only one block but multiple transaction
	for i := 0; i < len(cBlocks); i++ {
		var blockEnds sortableTxoSlice
		/*print("block: ")
		print(i)
		print("\n")
		print("block height: ")
		print(cBlocks[i].blockHeight)
		print("\n")
		print("num ttls: ")
		print(len(cBlocks[i].ttls))
		print("\n")*/
		//another for loop going through ttls. utxocounter increment for ttls not blocks
		for j:= 0; j < len(cBlocks[i].ttls);j++{
			allCounts  = allCounts + 1
			if(cBlocks[i].ttls[j] > 10){
				continue
			}
			var e txoEnd
			/*print("utxo counter: ")
			print(utxoCounter)
			print("\n")
			print("curr ttl: ")
			print(cBlocks[i].ttls[j])
			print("\n")*/
			e = txoEnd{
				txoIdx: utxoCounter,
				end : cBlocks[i].blockHeight + cBlocks[i].ttls[j],
			}
			utxoCounter++
			blockEnds = append(blockEnds, e)
		}
		//print("finished adding\n")
		//sortStart := time.Now()
		sort.SliceStable(blockEnds,func(i, j int) bool {
			return blockEnds[i].end < blockEnds[j].end
		})
		//print("sorted \n")
		clairSlice = mergeSortedSlices(clairSlice, blockEnds)
		//print("merged\n")
		//preLen := len(clairSlice)
		var remembers sortableTxoSlice
		//height blockHeight
		remembers, clairSlice = SplitAfter(clairSlice, cBlocks[i].blockHeight)
		//print("split\n")
		allRemembers = mergeSortedSlices(allRemembers, remembers)
		//postLen := len(clairSlice)
		if(len(clairSlice)>maxmem){
			clairSlice = clairSlice[:maxmem]
		}
		print("num remembers ")
		print(len(remembers))
		print("\n")
		//add counter that cumulatively counts how many we are remembering(i.e. density of schedule)
		if len(remembers) > 0 {
			for _, r := range remembers {
				//scheduleSlice[r.txoIdx] = 1
				assertBitInRam(r.txoIdx, scheduleSlice)
				//err := assertBitInFile(r.txoIdx, scheduleFile)
				// if err != nil {
				// 	fmt.Printf("assertBitInFile error\n")
				// 	return err
				// }
			}
		}
	}
	return allCounts,scheduleSlice, clairSlice, allRemembers, nil
}
func genClair(cBlocks []cBlock, scheduleSlice []byte, clairSlice sortableTxoSlice, maxmem int) (uint32, []byte, sortableTxoSlice, sortableTxoSlice, error){
	
	var utxoCounter uint32
	utxoCounter = 0
	var allCounts uint32
	allCounts = 0
	var allRemembers sortableTxoSlice
	//get rid of for loop. Only one block but multiple transaction
	for i := 0; i < len(cBlocks); i++ {
		var blockEnds sortableTxoSlice
		/*print("block: ")
		print(i)
		print("\n")
		print("block height: ")
		print(cBlocks[i].blockHeight)
		print("\n")
		print("num ttls: ")
		print(len(cBlocks[i].ttls))
		print("\n")*/
		//another for loop going through ttls. utxocounter increment for ttls not blocks
		for j:= 0; j < len(cBlocks[i].ttls);j++{
			allCounts += 1
			var e txoEnd
			/*print("utxo counter: ")
			print(utxoCounter)
			print("\n")
			print("curr ttl: ")
			print(cBlocks[i].ttls[j])
			print("\n")*/
			e = txoEnd{
				txoIdx: utxoCounter,
				end : cBlocks[i].blockHeight + cBlocks[i].ttls[j],
			}
			utxoCounter++
			blockEnds = append(blockEnds, e)
		}
		//print("finished adding\n")
		//sortStart := time.Now()
		sort.SliceStable(blockEnds,func(i, j int) bool {
			return blockEnds[i].end < blockEnds[j].end
		})
		//print("sorted \n")
		clairSlice = mergeSortedSlices(clairSlice, blockEnds)
		//print("merged\n")
		//preLen := len(clairSlice)
		var remembers sortableTxoSlice
		//height blockHeight
		remembers, clairSlice = SplitAfter(clairSlice, cBlocks[i].blockHeight)
		//print("split\n")
		allRemembers = mergeSortedSlices(allRemembers, remembers)
		//postLen := len(clairSlice)
		if(len(clairSlice)>maxmem){
			clairSlice = clairSlice[:maxmem]
		}
		print("num remembers ")
		print(len(remembers))
		print("\n")
		//add counter that cumulatively counts how many we are remembering(i.e. density of schedule)
		if len(remembers) > 0 {
			for _, r := range remembers {
				//scheduleSlice[r.txoIdx] = 1
				assertBitInRam(r.txoIdx, scheduleSlice)
				//err := assertBitInFile(r.txoIdx, scheduleFile)
				// if err != nil {
				// 	fmt.Printf("assertBitInFile error\n")
				// 	return err
				// }
			}
		}
	}
	return allCounts, scheduleSlice, clairSlice, allRemembers, nil
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
