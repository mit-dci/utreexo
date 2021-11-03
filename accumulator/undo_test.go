package accumulator

import (
	"bytes"
	"fmt"
	"math/rand"
	"reflect"
	"testing"
)

func TestUndoSerializeDeserialize(t *testing.T) {
	tests := []struct {
		name       string
		undo       UndoBlock
		serialized []byte
	}{
		{
			name: "testnet3 block 412",
			undo: UndoBlock{
				numAdds:   6,
				positions: []uint64{455, 459, 461, 464},
				hashes: []Hash{
					Hash{
						0x12, 0x53, 0x6b, 0x08, 0x63, 0x1b, 0x00, 0x3c,
						0xa7, 0x97, 0xc3, 0x94, 0x33, 0x3b, 0x7f, 0x98,
						0xd5, 0x4a, 0x7d, 0x36, 0x3c, 0x11, 0xc7, 0x44,
						0x92, 0x60, 0x72, 0xa1, 0xad, 0xba, 0x20, 0x0b,
					},
					Hash{
						0xf9, 0x0d, 0xea, 0x08, 0xc2, 0xcc, 0x6b, 0x71,
						0x54, 0xe4, 0x38, 0x94, 0x40, 0xbc, 0x37, 0xc0,
						0xc0, 0x99, 0x05, 0x11, 0x0d, 0x68, 0xd0, 0xf9,
						0x57, 0x43, 0x0f, 0xac, 0x98, 0xe4, 0x59, 0x30,
					},
					Hash{
						0xf0, 0xe2, 0xdc, 0x5e, 0xc0, 0x2e, 0x8f, 0x67,
						0x32, 0xd4, 0x06, 0x4e, 0xc9, 0xb6, 0x73, 0x86,
						0x62, 0xaf, 0xb0, 0x93, 0x69, 0xea, 0x9d, 0xfb,
						0xa9, 0x05, 0x98, 0x11, 0x87, 0x05, 0x43, 0x85,
					},
					Hash{
						0x88, 0x66, 0x53, 0xab, 0xd5, 0x16, 0x0f, 0xc5,
						0x91, 0x9e, 0x0e, 0x38, 0x5c, 0x0b, 0x83, 0x7e,
						0x97, 0xfb, 0xb9, 0x16, 0xed, 0x0a, 0x0d, 0xb6,
						0x20, 0xba, 0x6b, 0xfc, 0x2b, 0xbb, 0x7c, 0xff,
					},
				},
			},
		},
		{
			name: "testnet3 block 450",
			undo: UndoBlock{
				numAdds:   3,
				positions: []uint64{454, 474},
				hashes: []Hash{
					Hash{
						0x10, 0xd1, 0x0a, 0xf8, 0xf2, 0x0b, 0x14, 0xc6,
						0x12, 0xb1, 0x77, 0xa5, 0xaf, 0xd5, 0x44, 0x76,
						0x0f, 0xf5, 0xc4, 0x96, 0x18, 0xac, 0x91, 0x1c,
						0xa2, 0x43, 0xf4, 0x19, 0xf9, 0x31, 0xe1, 0x24,
					},
					Hash{
						0x0a, 0xb5, 0x31, 0x83, 0xf6, 0x1f, 0xdb, 0xf0,
						0xd1, 0x3d, 0x03, 0xd4, 0x4b, 0x44, 0x14, 0x0f,
						0xc3, 0x89, 0x9a, 0x28, 0x25, 0xf6, 0xc5, 0x8e,
						0xe7, 0x18, 0x69, 0x90, 0xe0, 0x85, 0xae, 0xfb,
					},
				},
			},
		},
	}

	for _, test := range tests {
		beforeSize := test.undo.SerializeSize()
		buf := make([]byte, 0, beforeSize)
		w := bytes.NewBuffer(buf)
		err := test.undo.Serialize(w)
		if err != nil {
			t.Fatal(err)
		}

		serializedBytes := w.Bytes()

		beforeBytes := make([]byte, len(serializedBytes))
		copy(beforeBytes, serializedBytes)

		r := bytes.NewReader(serializedBytes)
		afterUndo := new(UndoBlock)
		afterUndo.Deserialize(r)

		afterSize := afterUndo.SerializeSize()

		if beforeSize != afterSize {
			str := fmt.Errorf("Serialized sizes differ. Before:%dAfter:%d", beforeSize, afterSize)
			t.Error(str)
		}

		afterBuf := make([]byte, 0, afterSize)
		afterW := bytes.NewBuffer(afterBuf)
		err = afterUndo.Serialize(afterW)
		if err != nil {
			t.Fatal(err)
		}
		afterBytes := afterW.Bytes()

		if !bytes.Equal(beforeBytes[:], afterBytes[:]) {
			str := fmt.Errorf("Serialized bytes differ. Before:%x after:%x", beforeBytes, afterBytes)
			t.Error(str)
		}
	}
}

func TestUndoFixed(t *testing.T) {
	rand.Seed(2)

	// needs in-order
	err := undoAddDelOnce(6, 4, 4)
	if err != nil {
		t.Fatal(err)
	}
}

func TestUndoRandom(t *testing.T) {

	for z := int64(0); z < 100; z++ {
		// z := int64(11)
		rand.Seed(z)
		err := undoOnceRandom(20)
		if err != nil {
			fmt.Printf("rand seed %d\n", z)
			t.Fatal(err)
		}
	}
}

func TestUndoTest(t *testing.T) {
	rand.Seed(1)
	err := undoTestSimChain()
	if err != nil {
		t.Fatal(err)
	}
}

func undoOnceRandom(blocks int32) error {
	f := NewForest(RamForest, nil, "", 0)

	sc := newSimChain(0x07)
	sc.lookahead = 0
	for b := int32(0); b < blocks; b++ {

		adds, durations, delHashes := sc.NextBlock(rand.Uint32() & 0x03)

		if verbose {
			fmt.Printf("\t\tblock %d del %d add %d - %s\n",
				sc.blockHeight, len(delHashes), len(adds), f.Stats())
		}

		bp, err := f.ProveBatch(delHashes)
		if err != nil {
			return err
		}
		beforeRoot := f.getRoots()
		ub, err := f.Modify(adds, bp.Targets)
		if err != nil {
			return err
		}
		if verbose {
			fmt.Print(f.ToString())
			fmt.Print(sc.ttlString())

			for h, p := range f.positionMap {
				fmt.Printf("%x@%d ", h[:4], p)
			}
		}
		err = f.PosMapSanity()
		if err != nil {
			return err
		}

		//undo every 3rd block
		if b%3 == 2 {
			if verbose {
				fmt.Print(ub.ToString())
			}
			err := f.Undo(*ub)
			if err != nil {
				return err
			}
			if verbose {
				fmt.Print("\n post undo map: ")
				for h, p := range f.positionMap {
					fmt.Printf("%x@%d ", h[:4], p)
				}
			}
			sc.BackOne(adds, durations, delHashes)
			afterRoot := f.getRoots()
			if !reflect.DeepEqual(beforeRoot, afterRoot) {
				return fmt.Errorf("undo mismatch")
			}
		}

	}
	if verbose {
		fmt.Printf("\n")
	}
	return nil
}

func undoAddDelOnce(numStart, numAdds, numDels uint32) error {
	f := NewForest(RamForest, nil, "", 0)
	sc := newSimChain(0xff)

	// --------------- block 0
	// make starting forest with numStart leaves, and store tops
	adds, _, _ := sc.NextBlock(numStart)
	fmt.Printf("adding %d leaves\n", numStart)
	_, err := f.Modify(adds, nil)
	if err != nil {
		return err
	}
	fmt.Printf(f.ToString())
	beforeTops := f.getRoots()
	for i, h := range beforeTops {
		fmt.Printf("beforeTops %d %x\n", i, h)
	}

	// ---------------- block 1
	// just delete from the left side for now.  Can try deleting scattered
	// randomly later
	delHashes := make([]Hash, numDels)
	for i, _ := range delHashes {
		delHashes[i] = adds[i].Hash
	}
	// get some more adds; the dels we already got
	adds, _, _ = sc.NextBlock(numAdds)

	bp, err := f.ProveBatch(delHashes)
	if err != nil {
		return err
	}

	fmt.Printf("block 1 add %d rem %d\n", numAdds, numDels)

	ub, err := f.Modify(adds, bp.Targets)
	if err != nil {
		return err
	}
	fmt.Print(f.ToString())
	fmt.Print(ub.ToString())
	afterTops := f.getRoots()
	for i, h := range afterTops {
		fmt.Printf("afterTops %d %x\n", i, h)
	}

	err = f.Undo(*ub)
	if err != nil {
		return err
	}

	undoneTops := f.getRoots()
	for i, h := range undoneTops {
		fmt.Printf("undoneTops %d %x\n", i, h)
	}
	for h, p := range f.positionMap {
		fmt.Printf("%x@%d ", h[:4], p)
	}
	fmt.Printf("tops: ")
	for i, _ := range beforeTops {
		fmt.Printf("pre %04x post %04x ", beforeTops[i][:4], undoneTops[i][:4])
		if undoneTops[i] != beforeTops[i] {
			return fmt.Errorf("block %d top %d mismatch, pre %x post %x",
				sc.blockHeight, i, beforeTops[i][:4], undoneTops[i][:4])
		}
	}
	fmt.Printf("\n")

	return nil
}

func undoTestSimChain() error {

	sc := newSimChain(7)
	sc.NextBlock(3)
	sc.NextBlock(3)
	sc.NextBlock(3)
	fmt.Printf(sc.ttlString())
	l1, dur, h1 := sc.NextBlock(3)
	fmt.Printf(sc.ttlString())
	sc.BackOne(l1, dur, h1)
	fmt.Printf(sc.ttlString())
	return nil
}
