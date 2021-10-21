package bridgenode

import (
	"encoding/binary"
	"fmt"
	"os"
	"sort"
)

func BuildClairvoyantSchedule(cfg *Config, sig chan bool) error {

	// Open the offset file and ttl file which have already been saved
	ttlOffsetFile, err := os.Open(cfg.UtreeDir.TtlDir.OffsetFile)
	if err != nil {
		return err
	}
	ttlFile, err := os.Open(cfg.UtreeDir.TtlDir.ttlsetFile)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(
		cfg.UtreeDir.TtlDir.ClairvoyFile, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	fmt.Println("Done opening files")
	var numCBlocks = 200

	// the format of the offset file is a series of 8-byte big-endian integers
	// which say the byte that every block starts at.  To get the starting point
	// of the 100th block, go to the 800th byte of the offset file, and read
	// an int64.  Then seek to that location within the TTL file for the
	// series of 4-byte TTL values of block 100.

	// example: read block 208 and report the ttls
	s, _ := ttlFile.Stat()
	size := int64(s.Size())
	//size is stat size of ttlfile divide by 4 divide by 8
	err = f.Truncate(size)
	if err != nil {
		return err
	}
	/*fmt.Println("Done seeking")
	_, err = f.Write([]byte{0x00})
	if err != nil {
		return err
	}*/
	fmt.Println("Done writing inital ones")
	defer f.Close()
	//numCBlocks = height of ttloffset file over 8
	ttlOffsets, _ := ttlOffsetFile.Stat()
	numCBlocks = int(ttlOffsets.Size() / 8)
	var utxoCounter uint32 = 0
	maxmems := []int{5000}
	clairSlices := make([][]txoEnd, len(maxmems))
	for height := 0; height < numCBlocks; height++ {
		//goes through all blocks
		// offset is the position in the ttlFile where block starts
		var offset int64
		//fmt.Println("height:" + fmt.Sprint(height))
		// seek to the right place in the offset file
		_, err = ttlOffsetFile.Seek(int64(height)*8, 0)
		if err != nil {
			return err
		}

		// read the offset data
		err = binary.Read(ttlOffsetFile, binary.BigEndian, &offset)
		if err != nil {
			return err
		}
		var nextOffset int64
		// read the offset data
		err = binary.Read(ttlOffsetFile, binary.BigEndian, &nextOffset)
		if err != nil {
			return err
		}

		// seek to the block start in the ttl file
		_, err = ttlFile.Seek(offset, 0)
		if err != nil {
			return err
		}

		// read number of ttls
		/*var ttlsInBlock int32
		err = binary.Read(ttlFile, binary.BigEndian, &ttlsInBlock)
		if err != nil {
			return err
		}*/
		ttlsInBlock := (nextOffset - offset) / 4
		ttls := make([]int32, ttlsInBlock)
		for i, _ := range ttls {
			binary.Read(ttlFile, binary.BigEndian, &ttls[i])
		}

		// print out those ttls
		/*for i, t := range ttls {
			fmt.Printf("height %d ttl %d = %d\n", height, i, t)
		}*/

		//var allCounts uint32 = 0
		//numRemembers := make([]int, len(maxmems))
		//for i := 0; i < len(ttls); i++ {
		//goes through every ttl
		var blockEnds []txoEnd
		//another for loop going through ttls. utxocounter increment for ttls not blocks
		for j := 0; j < len(ttls); j++ {
			if ttls[j] >= 2147483600 {
				//invalid output, so skip and don't count
				continue
			}
			//allCounts += 1
			var e txoEnd = txoEnd{
				txoIdx: utxoCounter,
				end:    int32(height) + ttls[j],
			}
			utxoCounter++
			blockEnds = append(blockEnds, e)
		}
		if height%100 == 0 {
			fmt.Printf("On block: %d; ttls in block: %d; length of clair slice: %d; utxoCounter: %d, block end length: %d \n",
				height, ttlsInBlock, len(clairSlices[0]), utxoCounter, len(blockEnds))
		}
		sort.SliceStable(blockEnds, func(i, j int) bool {
			return blockEnds[i].end < blockEnds[j].end
		})
		for j := 0; j < len(maxmems); j++ {
			clairSlices[j] = mergeSortedSlices(clairSlices[j], blockEnds)
			var remembers []txoEnd
			remembers, clairSlices[j] =
				SplitAfter(clairSlices[j], int32(height))

			for k := 0; k < len(remembers); k++ {
				currTxo := remembers[k]
				ind := currTxo.txoIdx
				//fmt.Println("We remembered txo; asserting in file : " + fmt.Sprint(ind))
				err := assertBitInFile(ind, f)
				if err != nil {
					//fmt.Println("error")
					return err
				}
				//fmt.Println("done")
			}
			if len(clairSlices[j]) > maxmems[j] {
				clairSlices[j] = clairSlices[j][:maxmems[j]]
			}
		}
		//}
	}
	fmt.Println("done with genschedule")
	return nil
}
