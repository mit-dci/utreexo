package clair

import (
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/mit-dci/lit/wire"
	"github.com/mit-dci/utreexo/cmd/ibdsim"
	"github.com/mit-dci/utreexo/cmd/utils"
	//"github.com/mit-dci/utreexo/utreexo"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
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
	end    uint32 // when it dies (block height)
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
func SplitAfter(s sortableTxoSlice, h uint32) (top, bottom sortableTxoSlice) {
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

func Clairvoy(ttldb string, offsetfile string, mem string, sig chan bool) error {
	fmt.Printf("genclair - builds clairvoyant caching schedule\n")

	//Channel to alert the main loop to break
	stopGoing := make(chan bool, 1)

	//Channel to alert stopTxottl it's ok to exit
	done := make(chan bool, 1)

	go stopClairvoy(sig, stopGoing, done)

	scheduleSlice := make([]byte, 125000000)

	// scheduleFile, err := os.Create("schedule.clr")
	// }
	// we should know how many utxos there are before starting this, and allocate
	// (truncate!? weird) that many bits (/8 for bytes)
	// err = scheduleFile.Truncate(125000000) // 12.5MB for testnet (guess)
	// if err != nil {
	// 	return err
	// }

	// the index file will be useful later for ibdsim, if you have a block
	// height and need to know where in the clair schedule you are.
	// indexFile, err := os.Create("index.clr")
	// if err != nil {
	// 	return err
	// }

	// defer scheduleFile.Close()

	// open ttl database
	o := new(opt.Options)
	o.CompactionTableSizeMultiplier = 8
	o.ReadOnly = true
	lvdb, err := leveldb.OpenFile(ttldb, o)
	if err != nil {
		panic(err)
	}
	defer lvdb.Close()

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

	maxmem, err := strconv.Atoi(mem)
	if err != nil || maxmem == 0 {
		return fmt.Errorf("usage: clair memorysize  (eg ./clair 3000)\n")

	}

	var utxoCounter uint32
	var height int
	var clairSlice, remembers sortableTxoSlice
	// _, err = indexFile.WriteAt(U32tB(0), 0) // first 0 bytes because blocks start at 1
	// if err != nil {
	// 	return err
	// }

	// To send/receive blocks from blockreader()
	bchan := make(chan simutil.BlockToWrite, 20)

	//bool for stopping the scanner.Scan loop
	var stop bool

	// Reads block asynchronously from .dat files
	go simutil.BlockReader(bchan, currentOffsetHeight, height, offsetfile)

	for ; height != currentOffsetHeight && stop != true; height++ {

		b := <-bchan

		scheduleSlice, clairSlice, remembers, err = genClair(b.Txs, uint32(b.Height),
			&utxoCounter, maxmem, scheduleSlice, clairSlice, remembers, lvdb)
		if err != nil {
			panic(err)
		}
		//if height%1000 == 0 {
		//	fmt.Println("On height:", height+1)
		//}
		//Check if stopSig is no longer false
		//stop = true makes the loop exit
		select {
		case stop = <-stopGoing:
		default:
		}
	}
	fmt.Printf("done\n")

	fileString := fmt.Sprintf("schedule%dpos.clr", maxmem)
	ioutil.WriteFile(fileString, scheduleSlice, 0644)
	fmt.Println("utxoCounter", utxoCounter)
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

//genClair generates the clair caching schedule
func genClair(
	tx []*wire.MsgTx,
	height uint32,
	utxoCounter *uint32,
	maxmem int,
	scheduleSlice []byte,
	clairSlice sortableTxoSlice,
	remembers sortableTxoSlice,
	lvdb *leveldb.DB) ([]byte, sortableTxoSlice, sortableTxoSlice, error) {

	blocktxs := []*simutil.Txotx{new(simutil.Txotx)}
	var blockEnds sortableTxoSlice
	var sortTime time.Duration
	startTime := time.Now()

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
				// mark all unspendables true
				blocktxs[len(blocktxs)-1].Unspendable[i] = true
			} else {
				//txid := tx.TxHash().String()
				blocktxs[len(blocktxs)-1].Outputtxid = txhash.String()
				blocktxs[len(blocktxs)-1].DeathHeights = make([]uint32, numoutputs)
			}
		}

		// done with this Txotx, make the next one and append
		blocktxs = append(blocktxs, new(simutil.Txotx))

	}
	//TODO Get rid of this. This eats up cpu
	//we started a tx but shouldn't have
	blocktxs = blocktxs[:len(blocktxs)-1]
	// call function to make all the db lookups and find deathheights
	ibdsim.LookupBlock(blocktxs, lvdb)

	for _, blocktx := range blocktxs {
		// genTXOEndHeight appends to blockEnds
		ends, err := genTXOEndHeight(blocktx, utxoCounter, height+1)
		if err != nil {
			panic(err)
		}
		for _, end := range ends {
			blockEnds = append(blockEnds, end)
		}

	}

	// append & sort
	sortStart := time.Now()
	// presort the smaller slice
	sort.Sort(blockEnds)
	// merge sorted
	clairSlice = mergeSortedSlices(clairSlice, blockEnds)
	sortTime += time.Now().Sub(sortStart)

	// clear blockEnds
	//blockEnds = sortableTxoSlice{}

	// chop off the beginning: that's the stuff that's memorable
	preLen := len(clairSlice)
	remembers, clairSlice = SplitAfter(clairSlice, height+1)
	postLen := len(clairSlice)
	if preLen != len(remembers)+postLen {
		return nil, nil, nil, fmt.Errorf("h %d preLen %d remembers %d postlen %d\n",
			height+1, preLen, len(remembers), postLen)
	}

	// chop off the end, that's stuff that is forgettable
	if len(clairSlice) > maxmem {
		//				forgets := clairSlice[maxmem:]
		// fmt.Printf("\tblock %d forget %d\n",
		// height, len(clairSlice)-maxmem)
		clairSlice = clairSlice[:maxmem]

		//				for _, f := range forgets {
		//					fmt.Printf("%d ", f.txoIdx)
		//				}
		//				fmt.Printf("\n")
	}

	// expand index file and schedule file (with lots of 0s)
	// _, err := indexFile.WriteAt(
	// 	U32tB(utxoCounter-txosThisBlock), int64(height)*4)
	// if err != nil {
	// 	return err
	// }

	// writing remembers is trickier; check in
	if len(remembers) > 0 {
		for _, r := range remembers {
			assertBitInRam(r.txoIdx, scheduleSlice)
			// err = assertBitInFile(r.txoIdx, scheduleFile)
			// if err != nil {
			// 	fmt.Printf("assertBitInFile error\n")
			// 	return err
			// }
		}

	}

	if height%10 == 0 {
		fmt.Printf("all %.2f sort %.2f ",
			time.Now().Sub(startTime).Seconds(),
			sortTime.Seconds())
		fmt.Printf("h %d txo %d clairSlice %d ",
			height+1, *utxoCounter, len(clairSlice))
		if len(clairSlice) > 0 {
			fmt.Printf("first %d:%d last %d:%d\n",
				clairSlice[0].txoIdx,
				clairSlice[0].end,
				clairSlice[len(clairSlice)-1].txoIdx,
				clairSlice[len(clairSlice)-1].end)
		} else {
			fmt.Printf("\n")
		}
	}
	return scheduleSlice, clairSlice, remembers, nil
}

// plusLine reads in a line of text, generates a utxo leaf, and determines
// if this is a leaf to remember or not.
// Modifies the blockEnds var that was passed in
func genTXOEndHeight(tx *simutil.Txotx, utxoCounter *uint32, height uint32) (sortableTxoSlice, error) {
	blockEnds := sortableTxoSlice{}
	for i := 0; i < len(tx.DeathHeights); i++ {
		if tx.Unspendable[i] == true {
			continue
		}
		// Skip all the same block spends
		if tx.DeathHeights[i]-height == 0 {
			continue
		}
		// Deatheight 0 means it's a UTXO. Don't remember UTXOs.
		if tx.DeathHeights[i] == 0 {
			e := txoEnd{
				txoIdx: *utxoCounter,
				end:    uint32(1 << 30),
			}
			*utxoCounter++
			blockEnds = append(blockEnds, e)
		} else {
			e := txoEnd{
				txoIdx: *utxoCounter,
				end:    tx.DeathHeights[i],
			}
			*utxoCounter++
			blockEnds = append(blockEnds, e)
		}

	}
	return blockEnds, nil
}

// This is copied from utreexo utils, and in this cases there will be no
// duplicates, so that part is removed.  Uses sortableTxoSlices.

// mergeSortedSlices takes two slices (of uint64s; though this seems
// genericizable in that it's just < and > operators) and merges them into
// a signle sorted slice, discarding duplicates.
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

func stopClairvoy(sig chan bool, stopGoing chan bool, done chan bool) {
	<-sig
	fmt.Println("Exiting...")

	//Tell Runibd() to finish the block it's working on
	stopGoing <- true

	//Wait Runidb() says it's ok to quit
	<-done
	os.Exit(0)
}
