package tree

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/mit-dci/utreexo/util"
)

func TestPollardFullRand(t *testing.T) {
	for z := 0; z < 30; z++ {
		// z := 1
		rand.Seed(int64(z))
		fmt.Printf("randseed %d\n", z)
		err := pollardFullRandomRemember(20)
		if err != nil {
			fmt.Printf("randseed %d\n", z)
			t.Fatal(err)
		}
	}
}

func pollardFullRandomRemember(blocks int32) error {

	// ffile, err := os.Create("/dev/shm/forfile")
	// if err != nil {
	// return err
	// }

	var fp, p Pollard
	fp = NewFullPollard()

	// p.Minleaves = 0

	sn := util.NewSimChain(0x07)
	sn.Lookahead = 400
	for b := int32(0); b < blocks; b++ {
		adds, _, delHashes := sn.NextBlock(rand.Uint32() & 0x03)

		fmt.Printf("\t\t\tstart block %d del %d add %d - %s\n",
			sn.BlockHeight, len(delHashes), len(adds), p.Stats())

		// get proof for these deletions (with respect to prev block)
		bp, err := fp.ProveBatch(delHashes)
		if err != nil {
			return err
		}

		// verify proofs on rad node
		err = p.IngestBatchProof(bp)
		if err != nil {
			return err
		}
		fmt.Printf("del %v\n", bp.Targets)

		// apply adds and deletes to the bridge node (could do this whenever)
		err = fp.Modify(adds, bp.Targets)
		if err != nil {
			return err
		}
		// TODO fix: there is a leak in forest.Modify where sometimes
		// the position map doesn't clear out and a hash that doesn't exist
		// any more will be stuck in the positionMap.  Wastes a bit of memory
		// and seems to happen when there are moves to and from a location
		// Should fix but can leave it for now.

		err = fp.PosMapSanity()
		if err != nil {
			fmt.Printf(fp.ToString())
			return err
		}

		// apply adds / dels to pollard
		err = p.Modify(adds, bp.Targets)
		if err != nil {
			return err
		}

		fmt.Printf("pol postadd %s", p.ToString())

		fmt.Printf("fulpol postadd %s", fp.ToString())

		fullTops := fp.topHashesReverse()
		polTops := p.topHashesReverse()

		// check that tops match
		if len(fullTops) != len(polTops) {
			return fmt.Errorf("block %d fulpol %d tops, pol %d tops",
				sn.BlockHeight, len(fullTops), len(polTops))
		}
		fmt.Printf("top matching: ")
		for i, ft := range fullTops {
			fmt.Printf("fp %04x p %04x ", ft[:4], polTops[i][:4])
			if ft != polTops[i] {
				return fmt.Errorf("block %d top %d mismatch, fulpol %x pol %x",
					sn.BlockHeight, i, ft[:4], polTops[i][:4])
			}
		}
		fmt.Printf("\n")
	}

	return nil
}
