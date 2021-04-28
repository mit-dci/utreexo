package main

import (
	"fmt"
	"sort"
)

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
func genClairResetSlice(allCBlocks []cBlock, resetSize []int, maxmems []int) (uint32, []int, error) {
	//scheduleSlice := make([]byte, 125000000)
	resetSlices := make([]sortableTxoSlice, len(resetSize))
	var utxoCounter uint32
	utxoCounter = 0
	var allCounts uint32
	allCounts = 0
	numRemembers := make([]int, len(resetSize))
	for i := 0; i < len(allCBlocks); i++ {
		/*currBlocks,err := getCBlocks(int32(i)+1,1)
		  if(err != nil){
		      panic(err)
		  }
		  currBlock := currBlocks[0]*/
		for j := 0; j < len(resetSize); j++ {
			if i%resetSize[j] == 0 {
				resetSlices[j] = resetSlices[j][:0]
			}
		}
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
	fmt.Println("total number of remembers for CLAIRVOY:", numRemembers)
	fmt.Println("all Blocks: ", allCounts)
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
