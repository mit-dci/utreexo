package accumulator

import (
	"fmt"
	"testing"
)

func TestIsAncestor(t *testing.T) {
	fmt.Println(isAncestor(62, 8, 5))
	fmt.Println(isAncestor(62, 0, 5))
	fmt.Println(isAncestor(32, 0, 5))
	fmt.Println(isAncestor(57, 36, 5))

	fmt.Println()

	fmt.Println(isAncestor(61, 0, 5))
	fmt.Println(isAncestor(57, 0, 5))
	fmt.Println(isAncestor(50, 0, 5))
}

func TestSubTreeRows(t *testing.T) {
	subTree := detectSubTreeRows(8, 15, 4)
	fmt.Println(subTree)

	rootPresent := 15&(1<<subTree) != 0
	rootPos := rootPosition(15, subTree, 4)

	if rootPresent {
		fmt.Println("root pos is ", rootPos)
	}
}

func TestProofPositionsSwapless(t *testing.T) {
	proofPositions := NewPositionList()
	defer proofPositions.Free()

	targets := []uint64{4, 9}
	numLeaves := uint64(8)
	forestRows := uint8(3)

	//ProofPositionsSwapless(targets, numLeaves, forestRows, &proofPositions.list)
	ProofPositions(targets, numLeaves, forestRows, &proofPositions.list)

	fmt.Println(proofPositions.list)
}

func TestTreeRows(t *testing.T) {
	// Test all the powers of 2
	for i := uint8(1); i <= 63; i++ {
		nLeaves := uint64(1 << i)
		Orig := treeRowsOrig(nLeaves)
		New := treeRows(nLeaves)
		if Orig != New {
			fmt.Printf("for n: %d;orig is %d. new is %d\n", nLeaves, Orig, New)
			t.Fatal("treeRows and treeRowsOrig are not the same")
		}

	}
	// Test billion leaves
	for n := uint64(0); n <= 100000000; n++ {
		Orig := treeRowsOrig(n)
		New := treeRows(n)
		if Orig != New {
			fmt.Printf("for n: %d;orig is %d. new is %d\n", n, Orig, New)
			t.Fatal("treeRows and treeRowsOrig are not the same")
		}
	}
}

// This is the orginal code for getting treeRows. The new function is tested
// against it.
func treeRowsOrig(n uint64) (e uint8) {
	// Works by iteratations of shifting left until greater than n
	for ; (1 << e) < n; e++ {
	}
	return
}

func BenchmarkTreeRows_HunThou(b *testing.B) { benchmarkTreeRows(100000, b) }
func BenchmarkTreeRows_Mil(b *testing.B)     { benchmarkTreeRows(1000000, b) }
func BenchmarkTreeRows_Bil(b *testing.B)     { benchmarkTreeRows(10000000, b) }
func BenchmarkTreeRows_Tril(b *testing.B)    { benchmarkTreeRows(100000000, b) }

func BenchmarkOrigTreeRows_HunThou(b *testing.B) { benchmarkTreeRowsOrig(100000, b) }
func BenchmarkOrigTreeRows_Mil(b *testing.B)     { benchmarkTreeRowsOrig(1000000, b) }
func BenchmarkOrigTreeRows_Bil(b *testing.B)     { benchmarkTreeRowsOrig(10000000, b) }
func BenchmarkOrigTreeRows_Tril(b *testing.B)    { benchmarkTreeRowsOrig(100000000, b) }

func benchmarkTreeRows(i uint64, b *testing.B) {
	for n := uint64(1000000); n < i+1000000; n++ {
		treeRows(n)
	}
}

func benchmarkTreeRowsOrig(i uint64, b *testing.B) {
	for n := uint64(1000000); n < i+1000000; n++ {
		treeRowsOrig(n)
	}
}
