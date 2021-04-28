package main

func LookBehindResetSlice(allCBlocks []cBlock, resetSizes []int, maxmems []int) []int {
	cache := make([][]int, len(resetSizes))
	deletion := make([][][]int, len(resetSizes))
	for i := 0; i < len(resetSizes); i++ {
		cache[i] = make([]int, 0)
		deletion[i] = make([][]int, resetSizes[i])
		for j := 0; j < resetSizes[i]; j++ {
			deletion[i][j] = make([]int, 0)
		}
	}
	memPointers := make([]int, len(maxmems))
	utxoCounter := 0
	totalRemembers := make([]int, len(maxmems))
	for i := 0; i < len(allCBlocks); i++ {
		for j := 0; j < len(resetSizes); j++ {
			if i%resetSizes[j] == 0 {
				cache[j] = cache[j][:0]
				deletion[j] = make([][]int, resetSizes[j])
				for k := 0; k < resetSizes[j]; k++ {
					deletion[j][k] = make([]int, 0)
				}
			}
		}
		for j := 0; j < len(allCBlocks[i].ttls); j++ {
			//if lives too long and we don't look at that block to delete, then just ignore
			for k := 0; k < len(resetSizes); k++ {
				if allCBlocks[i].ttls[j] >= int32(len(deletion[k])) {
					continue
				}
				deletion[k][allCBlocks[i].ttls[j]] = append(deletion[k][allCBlocks[i].ttls[j]], utxoCounter)
				cache[k] = append(cache[k], utxoCounter)
			}
			utxoCounter += 1
		}
		numRemembers := make([]int, len(resetSizes))
		// The way cache and deletion are built, both should always be sorted
		for j := 0; j < len(resetSizes); j++ {
			currDelPos := len(deletion[j][0]) - 1
			currCachePos := len(cache[j]) - 1
			for currDelPos >= 0 && currCachePos >= 0 {
				for currDelPos >= 0 && deletion[j][0][currDelPos] > cache[j][currCachePos] {
					//continue incrementing deletion pos if cache already passed it
					currDelPos -= 1
				}
				if currDelPos < 0 {
					break
				}
				if deletion[j][0][currDelPos] == cache[j][currCachePos] {
					// we found it! This means we remembered it and we can increment
					if memPointers[j] <= currCachePos {
						// this is remembered for this specific size
						numRemembers[j] += 1
						totalRemembers[j] += 1
					}
				}
				currDelPos -= 1
				//remove from cache
				cache[j] = append(cache[j][:currCachePos], cache[j][currCachePos+1:]...)
				currCachePos -= 1
			}
			deletion[j] = deletion[j][1:]
			trimPos := len(cache[j]) - maxmems[j]
			if trimPos > 0 {
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
