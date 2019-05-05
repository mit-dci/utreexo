package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
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
	return
}

func main() {

	fmt.Printf("clair - builds clairvoyant caching schedule")
	err := clairvoy()
	if err != nil {
		panic(err)
	}
}

func clairvoy() error {
	txofile, err := os.OpenFile("ttl.testnet3.txos", os.O_RDONLY, 0600)
	if err != nil {
		return err
	}

	defer txofile.Close()
	scanner := bufio.NewScanner(txofile)
	scanner.Buffer(make([]byte, 1<<20), 1<<20) // 1MB should be enough?

	maxmem := 1000

	var blockEnds sortableTxoSlice

	var clairSlice, remembers sortableTxoSlice

	var utxoCounter uint32
	var height uint32
	height = 1

	for scanner.Scan() {
		switch scanner.Text()[0] {
		case '-':
			// do nothing?
		case '+':

			endHeights, err := plusLine(scanner.Text())
			if err != nil {
				return err
			}
			blockEnds = make([]txoEnd, len(endHeights))
			for i, eh := range endHeights {
				blockEnds[i].txoIdx = utxoCounter
				utxoCounter++
				blockEnds[i].end = height + eh
			}

		case 'h':

			//			fmt.Printf("h %d clairslice ", height)
			//			for _, u := range clairSlice {
			//				fmt.Printf("%d:%d, ", u.txoIdx, u.end)
			//			}
			//			fmt.Printf("\n")

			// append & sort
			clairSlice.MergeSort(blockEnds)

			// chop off the beginning: that's the stuff that's memorable
			remembers, clairSlice = SplitAfter(clairSlice, height)

			// chop off the end, that's stuff that is forgettable
			if len(clairSlice) > maxmem {
				forgets := clairSlice[maxmem:]
				clairSlice = clairSlice[:maxmem]
				fmt.Printf("forget ")
				for _, f := range forgets {
					fmt.Printf("%d ", f.txoIdx)
				}
				fmt.Printf("\n")
			}

			if len(remembers) > 0 {
				fmt.Printf("h %d remember utxos ", height)
				for _, r := range remembers {
					fmt.Printf("%d ", r.txoIdx)
				}
				fmt.Printf("\n")
			}

			//			fmt.Printf("h %d len(clairSlice) %d len(blockEnds) %d\n",
			//				height, len(clairSlice), len(blockEnds))

			height++

		default:
			panic("unknown string")

		}
	}

	return nil
}

// like the plusline in ibdsim.  Should merge with that.
// this one only returns a slice of the expiry times for the txos, but no other
// txo info.
func plusLine(s string) ([]uint32, error) {
	//	fmt.Printf("%s\n", s)
	parts := strings.Split(s[1:], ";")
	if len(parts) < 2 {
		return nil, fmt.Errorf("line %s has no ; in it", s)
	}
	postsemicolon := parts[1]

	indicatorHalves := strings.Split(postsemicolon, "x")
	ttldata := indicatorHalves[1]
	ttlascii := strings.Split(ttldata, ",")
	// the last one is always empty as there's a trailing ,
	ttlval := make([]uint32, len(ttlascii)-1)
	for i, _ := range ttlval {
		if ttlascii[i] == "s" {
			//	ttlval[i] = 0
			// 0 means don't remember it! so 1 million blocks later
			ttlval[i] = 1 << 20
			continue
		}

		val, err := strconv.Atoi(ttlascii[i])
		if err != nil {
			return nil, err
		}
		ttlval[i] = uint32(val)
	}

	txoIndicators := strings.Split(indicatorHalves[0], "z")

	numoutputs, err := strconv.Atoi(txoIndicators[0])
	if err != nil {
		return nil, err
	}
	if numoutputs != len(ttlval) {
		return nil, fmt.Errorf("%d outputs but %d ttl indicators",
			numoutputs, len(ttlval))
	}

	// numoutputs++ // for testnet3.txos

	unspend := make(map[int]bool)

	if len(txoIndicators) > 1 {
		unspendables := txoIndicators[1:]
		for _, zstring := range unspendables {
			n, err := strconv.Atoi(zstring)
			if err != nil {
				return nil, err
			}
			unspend[n] = true
		}
	}
	var ends []uint32
	for i := 0; i < numoutputs; i++ {
		if unspend[i] {
			continue
		}
		ends = append(ends, ttlval[i])
		// fmt.Printf("expire in\t%d remember %v\n", ttlval[i], addData.Remember)
	}

	return ends, nil
}
