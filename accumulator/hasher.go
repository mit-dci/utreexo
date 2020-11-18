package accumulator

import (
	"fmt"
	"runtime"
	"sync"
)

// The min number of hashes a hashWorker receives.
const MinHashesPerWorker = 100

// Used to pass slices of hashwork to the hash workers.
var hashWorkChan chan []*hashWork
var hashWorkWg *sync.WaitGroup

// hashableNode is the data needed to perform a hash
type hashableNode struct {
	sib, dest *polNode
	position  uint64 // doesn't really need to be there, but convenient for debugging
}

type hashWork struct {
	left, right, parent Hash
}

func hashWorker() {
	for {
		work, ok := <-hashWorkChan
		if !ok {
			return
		}

		for _, w := range work {
			w.parent = parentHash(w.left, w.right)
		}

		hashWorkWg.Done()
	}
}

var maxHashesPerRoutine int
var avgHashesPerRoutine int
var count int

func hashRow(work []*hashWork) {
	if count != 0 && count%10000 == 0 {
		fmt.Println("hasRow:", maxHashesPerRoutine, float32(avgHashesPerRoutine)/float32(count))
	}

	numRoutines := runtime.NumCPU()
	workPerRoutine := len(work) / numRoutines
	if workPerRoutine > maxHashesPerRoutine {
		maxHashesPerRoutine = workPerRoutine
	}
	avgHashesPerRoutine += workPerRoutine
	count++

	if workPerRoutine < MinHashesPerWorker {
		// do hashes in sync because the scheduling overhead is not worth it
		// for a hand full of hashes.
		for _, w := range work {
			w.parent = parentHash(w.left, w.right)
		}
		return
	}

	// Split up the work load across numRoutines go routines.
	for i := 0; i < numRoutines && workPerRoutine > 0; i++ {
		hashWorkWg.Add(1)
		hashWorkChan <- work[i*workPerRoutine : (i+1)*workPerRoutine]
	}

	// Hash the rest
	workRest := len(work) % numRoutines
	if workRest > 0 {
		hashWorkWg.Add(1)
		hashWorkChan <- work[len(work)-workRest:]
	}

	if workPerRoutine > 0 || workRest > 0 {
		hashWorkWg.Wait()
	}
}
