package bridgenode

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"

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

func getCBlocks(cfg *Config, start int32, count int32) ([]cBlock, error) {
	// build cblock slice to return
	cblocks := make([]cBlock, count)
	var proofdir = cfg.UtreeDir.ProofDir

	// grab utreexo data and populate cblocks
	for i, _ := range cblocks {
		udataBytes, err := GetUDataBytesFromFile(
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

// assumes a sorted slice.  Splits on a "end" value, returns the low slice and
// leaves the higher "end" value sequence in place
func SplitAfter(s sortableTxoSlice, h int32) (top, bottom sortableTxoSlice) {
	for i, c := range s {
		if c.end > h {
			top = s[:i]    // return the beginning of the slice
			bottom = s[i:] // chop that part off
			break
		}
	}
	if top == nil {
		top = s
	}
	return
}

// flips bit n of a large file to 1.
func assertBitInFile(txoIdx uint32, scheduleFile *os.File) error {
	offset := int64(txoIdx / 8)
	b := make([]byte, 1)
	_, err := scheduleFile.ReadAt(b, offset)
	if err != nil {
		return err
	}
	b[0] = b[0] | 1<<(7-(txoIdx%8))
	//fmt.Println("number: " + fmt.Sprint(b[0]))
	//fmt.Println("offset: " + fmt.Sprint(offset))
	_, err = scheduleFile.WriteAt(b, offset)
	return err
}

func ScheduleFileToBoolArray(scheduleFile *os.File, startBitOffset int64, numberOfBits int64) (ans []bool, err error) {
	//s, _ := scheduleFile.Stat()
	//size := int64(s.Size())
	fmt.Println("schedule file called: start :", startBitOffset, " number of bits: ", numberOfBits)
	/*Make sure arguments are correct(nonnegative)*/
	ans = make([]bool, numberOfBits)
	for i := startBitOffset; i < startBitOffset+numberOfBits; i++ {
		//i is bit within file
		byteOffset := i / 8
		curr := make([]byte, 1)
		_, err := scheduleFile.ReadAt(curr, int64(byteOffset))
		if err != nil {
			fmt.Println("schedule file err: ", err.Error())
			if err == io.EOF {
				break
			}
		} else {
			fmt.Println("no error; read bit ", i, " : ", curr[0])
		}
		ans[i-startBitOffset] = (curr[0]&(1<<(7-(uint32(i)%8))) > 0)
	}
	return ans, err
}

func ScheduleFileToByteArray(scheduleFile *os.File, startBitOffset int64, numberOfBits int64) (ans []byte, err error) {
	//s, _ := scheduleFile.Stat()
	//size := int64(s.Size())
	fmt.Println("schedule file called: start :", startBitOffset, " number of bits: ", numberOfBits)
	/*Make sure arguments are correct(nonnegative)*/
	ans = make([]byte, numberOfBits)
	for i := startBitOffset; i < startBitOffset+numberOfBits; i++ {
		//i is bit within file
		//byteOffset := i / 8
		curr := make([]byte, 1)
		_, err := scheduleFile.ReadAt(curr, int64(i))
		if err != nil && err != io.EOF {
			break
		}
		ans[i] = curr[0]
		//ans[i-startBitOffset] = (curr[0]&(1<<(7-(uint32(i)%8))) > 0)
	}
	return ans, err
}

// flips a bit to 1.  Crashes if you're out of range.
func assertBitInRam(txoIdx uint32, scheduleSlice []byte) {
	offset := int64(txoIdx / 8)
	scheduleSlice[offset] |= 1 << (7 - (txoIdx % 8))
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
