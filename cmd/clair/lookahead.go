package main

import "fmt"

func LookAheadResetSlice(allCBlocks []cBlock, maxResets []int, maxHold int) ([]int, []int) {
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
		if i%100 == 0 {
			fmt.Println("On block: ", i)
		}
		for j := 0; j < len(maxResets); j++ {
			if i%maxResets[j] == 0 {
				prevSum[j] = 0
				currSum[j] = 0
				currRemembers[j] = make([]int, maxHold)
			}
		}
		numRemember := make([]int, len(maxResets))
		for j := 0; j < len(allCBlocks[i].ttls); j++ {
			for k := 0; k < len(maxResets); k++ {
				if allCBlocks[i].ttls[j] <= int32(maxHold) && int32(maxResets[k]-i) >= allCBlocks[i].ttls[j] {
					numRemember[k] += 1
				}
			}
		}
		for j := 0; j < len(maxResets); j++ {
			if i < maxHold {
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
