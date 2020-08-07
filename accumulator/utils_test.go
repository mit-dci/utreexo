package accumulator

import (
	"fmt"
	"testing"
)

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
