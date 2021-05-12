package main

import (
	"fmt"
	"testing"
)

// At the extreme end (0 memory or unlimited memory) all 3 strategies should
// give the same results (forgetting everything or remembering everything)
// This test makes sure they do in fact give the same results.
func TestExtremesAllStrategies(t *testing.T) {

	// first make 3 identical slices of random cBlocks.  The caching functions
	// might change the slices as they go, so we shouldn't re-use slices here
	behindSet, totalTTLs := getSimCBlocks(400)
	// for i, cb := range behindSet {
	// 	fmt.Printf("cb %d %v\n", i, cb.ttls)
	// 	var ones, twos int
	// 	for _, ttl := range cb.ttls {
	// 		if ttl == 2 {
	// 			twos++
	// 		}
	// 		if ttl == 1 {
	// 			ones++
	// 		}
	// 	}
	// 	fmt.Printf("ones: %d twos: %d\n", ones, twos)
	// }

	aheadSet := make([]cBlock, len(behindSet))
	copy(aheadSet, behindSet)
	clairvoySet := make([]cBlock, len(behindSet))
	copy(clairvoySet, behindSet)

	// and another 3 for the unlimited memory test
	behindSet2 := make([]cBlock, len(behindSet))
	copy(behindSet2, behindSet)
	clairvoySet2 := make([]cBlock, len(behindSet))
	copy(clairvoySet2, behindSet)
	aheadSet2 := make([]cBlock, len(behindSet))
	copy(aheadSet2, behindSet)

	// first test 0 memory ------------------------------------------------

	behind0Total, _ := LookBehind(behindSet, 0)
	fmt.Printf("0 mem look behind: %d\n", behind0Total)

	ahead0Total, _ := LookAhead(aheadSet, 0)
	fmt.Printf("0 mem look ahead: %d\n", ahead0Total)

	if behind0Total != ahead0Total {
		t.Fatalf("0 mem look ahead / look behind mismatch, %d vs %d remembered",
			behind0Total, ahead0Total)
	}

	// we know 0mem ahead and behind match, so match ahead with clairvoyant
	_, clair0Remember, err := genClair(clairvoySet, 0)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("0 mem clairvoyant: %d\n", clair0Remember)

	if ahead0Total != clair0Remember {
		t.Fatalf("0 mem look ahead / clairvoyant mismatch, %d vs %d remembered",
			ahead0Total, clair0Remember)
	}

	// next test unlimited memory -----------------------------------------

	behindUnlimTotal, _ := LookBehind(behindSet2, totalTTLs)
	fmt.Printf("unlimited mem look behind: %d\n", behindUnlimTotal)

	aheadUnlimTotal, _ := LookAhead(aheadSet2, len(aheadSet2))
	fmt.Printf("unlimited mem look ahead: %d\n", aheadUnlimTotal)

	if behindUnlimTotal != aheadUnlimTotal {
		t.Fatalf("unlim mem ahead / behind mismatch, %d vs %d remembered",
			behindUnlimTotal, aheadUnlimTotal)
	}

	// can now match ahead with clairvoyant if we get this far
	_, unlimClairRemember, err := genClair(clairvoySet, totalTTLs)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("unlimited mem clairvoyant: %d\n", unlimClairRemember)

	if aheadUnlimTotal != unlimClairRemember {
		t.Fatalf("unlimited mem ahead / clairvoyant mismatch, %d vs %d remembered",
			aheadUnlimTotal, unlimClairRemember)
	}
}

// make sure the multi slice and 1-pass agree
func TestSlicedStrategy(t *testing.T) {

	behindSet, _ := getSimCBlocks(333)
	BehindSet2 := make([]cBlock, len(behindSet))
	copy(BehindSet2, behindSet)

	aheadSet := make([]cBlock, len(behindSet))
	copy(aheadSet, behindSet)
	aheadSet2 := make([]cBlock, len(behindSet))
	copy(aheadSet2, behindSet)

	clairvoyantSet := make([]cBlock, len(behindSet))
	copy(clairvoyantSet, behindSet)
	clairvoyantSet2 := make([]cBlock, len(behindSet))
	copy(clairvoyantSet2, behindSet)

	_, cresult, err := genClair(clairvoyantSet, 100)
	if err != nil {
		t.Fatal(err)
	}
	_, cresultslice, err := genClairSlice(clairvoyantSet2, []int{100})
	if err != nil {
		t.Fatal(err)
	}
	if cresult != cresultslice[0] {
		t.Fatalf("clairvoyant non-set %d set %d", cresult, cresultslice[0])
	}

	aresult, _ := LookAhead(aheadSet, 8)
	aresultslice, _ := LookAheadSlice(aheadSet2, []int{8})
	if aresult != aresultslice[0] {
		t.Fatalf("ahead non-set %d set %d", aresult, aresultslice[0])
	}

	bresult, _ := LookBehind(behindSet, 10)
	bresultslice, _ := LookBehindSlice(BehindSet2, []int{10})
	if bresult != bresultslice[0] {
		t.Fatalf("behind non-set %d set %d", bresult, bresultslice[0])
	}
}

// func TestSliceExtremes(t *testing.T) {

// }
