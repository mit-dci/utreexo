package main

import (
	"math/rand"
	"testing"
)

func TestSliceDelete(t *testing.T) {

	slice1 := make([]int, 1000)
	for i, _ := range slice1 {
		slice1[i] = int(rand.Int31())
	}

	slice2 := make([]int, 1000)
	copy(slice2, slice1)

	deletionMap := make(map[int]bool)
	for i := 0; i < 100; i++ {
		del := int(rand.Int31n(1000))
		deletionMap[del] = true
	}

}
