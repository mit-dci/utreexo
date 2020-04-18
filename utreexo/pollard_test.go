package utreexo

import (
	"fmt"
	"log"
	"math/rand"
	"testing"
)

func TestPollardRand(t *testing.T) {
	logger := NewLogger(t)
	for z := 0; z < 30; z++ {
		// z := 11221
		// z := 55
		rand.Seed(int64(z))
		logger.Printf("randseed %d\n", z)
		err := pollardRandomRemember(logger, 20)
		if err != nil {
			logger.Printf("randseed %d\n", z)
			t.Fatal(err)
		}
	}
}

func TestPollardFixed(t *testing.T) {
	logger := NewLogger(t)
	rand.Seed(2)
	//	err := pollardMiscTest()
	//	if err != nil {
	//		t.Fatal(err)
	//	}
	//	for i := 6; i < 100; i++ {
	err := fixedPollard(logger, 7)
	if err != nil {
		t.Fatal(err)
	}
}

func pollardRandomRemember(logger *log.Logger, blocks int32) error {

	// ffile, err := os.Create("/dev/shm/forfile")
	// if err != nil {
	// return err
	// }

	f := NewForest(nil)

	var p Pollard
	p.loggers.SetLoggers(logger)

	// p.Minleaves = 0

	sn := NewSimChain(0x07)
	sn.lookahead = 400
	for b := int32(0); b < blocks; b++ {
		adds, delHashes := sn.NextBlock(rand.Uint32() & 0xff)

		logger.Printf("\t\t\tstart block %d del %d add %d - %s\n",
			sn.blockHeight, len(delHashes), len(adds), p.Stats())

		// get proof for these deletions (with respect to prev block)
		bp, err := f.ProveBlock(delHashes)
		if err != nil {
			return err
		}

		// verify proofs on rad node
		err = p.IngestBlockProof(bp)
		if err != nil {
			return err
		}
		logger.Printf("del %v\n", bp.Targets)

		// apply adds and deletes to the bridge node (could do this whenever)
		_, err = f.Modify(adds, bp.Targets)
		if err != nil {
			return err
		}
		// TODO fix: there is a leak in forest.Modify where sometimes
		// the position map doesn't clear out and a hash that doesn't exist
		// any more will be stuck in the positionMap.  Wastes a bit of memory
		// and seems to happen when there are moves to and from a location
		// Should fix but can leave it for now.

		err = f.sanity()
		if err != nil {
			logger.Printf("frs broke %s", f.ToString())
			for h, p := range f.positionMap {
				logger.Printf("%x@%d ", h[:4], p)
			}
			return err
		}
		err = f.PosMapSanity()
		if err != nil {
			logger.Printf(f.ToString())
			return err
		}

		// apply adds / dels to pollard
		err = p.Modify(adds, bp.Targets)
		if err != nil {
			return err
		}

		logger.Printf("pol postadd %s", p.ToString())

		logger.Printf("frs postadd %s", f.ToString())

		// check all leaves match
		if !p.equalToForestIfThere(f) {
			return fmt.Errorf("pollard and forest leaves differ")
		}

		fullTops := f.GetTops()
		polTops := p.topHashesReverse()

		// check that tops match
		if len(fullTops) != len(polTops) {
			return fmt.Errorf("block %d full %d tops, pol %d tops",
				sn.blockHeight, len(fullTops), len(polTops))
		}
		logger.Printf("top matching: ")
		for i, ft := range fullTops {
			logger.Printf("f %04x p %04x ", ft[:4], polTops[i][:4])
			if ft != polTops[i] {
				return fmt.Errorf("block %d top %d mismatch, full %x pol %x",
					sn.blockHeight, i, ft[:4], polTops[i][:4])
			}
		}
		logger.Printf("\n")
	}

	return nil
}

// fixedPollard adds and removes things in a non-random way
func fixedPollard(logger *log.Logger, leaves int32) error {
	logger.Printf("\t\tpollard test add %d remove 1\n", leaves)
	f := NewForest(nil)

	leafCounter := uint64(0)

	dels := []uint64{2, 5, 6}

	// they're all forgettable
	adds := make([]LeafTXO, leaves)

	// make a bunch of unique adds & make an expiry time and add em to
	// the TTL map
	for j, _ := range adds {
		adds[j].Hash[1] = uint8(leafCounter)
		adds[j].Hash[2] = uint8(leafCounter >> 8)
		adds[j].Hash[3] = uint8(leafCounter >> 16)
		adds[j].Hash[4] = uint8(leafCounter >> 24)
		adds[j].Hash[9] = uint8(0xff)

		// the first utxo added lives forever.
		// (prevents leaves from going to 0 which is buggy)
		adds[j].Remember = true
		leafCounter++
	}

	// apply adds and deletes to the bridge node (could do this whenever)
	_, err := f.Modify(adds, nil)
	if err != nil {
		return err
	}
	logger.Printf("forest  post del %s", f.ToString())

	var p Pollard
	p.loggers.SetLoggers(logger)

	err = p.add(adds)
	if err != nil {
		return err
	}

	logger.Printf("pollard post add %s", p.ToString())

	err = p.rem2(dels)
	if err != nil {
		return err
	}

	_, err = f.Modify(nil, dels)
	if err != nil {
		return err
	}
	logger.Printf("forest  post del %s", f.ToString())

	logger.Printf("pollard post del %s", p.ToString())

	if !p.equalToForest(f) {
		return fmt.Errorf("p != f (leaves)\n")
	}

	return nil
}
