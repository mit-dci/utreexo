//go:build gofuzz
// +build gofuzz

package accumulator

import (
	"bytes"
	"fmt"
	"reflect"
)

func undoOnceFuzzy(data *bytes.Buffer) error {
	f := NewForest(RamForest, nil, "", 0)

	seed0, err := data.ReadByte()
	if err != nil {
		return nil
	}
	seed1, err := data.ReadByte()
	if err != nil {
		return nil
	}
	seed := (int64(seed1) << 8) | int64(seed0)
	sc := newSimChainWithSeed(0x07, seed)
	if sc == nil {
		return nil
	}
	sc.lookahead = 0
	for b := int32(0); ; b++ {
		numAdds, err := data.ReadByte()
		if err != nil {
			break
		}
		numAdds &= 0x1f

		adds, durations, delHashes := sc.NextBlock(uint32(numAdds))

		bp, err := f.ProveBatch(delHashes)
		if err != nil {
			return err
		}
		beforeRoot := f.GetRoots()
		ub, err := f.Modify(adds, bp.Targets)
		if err != nil {
			return err
		}
		err = f.PosMapSanity()
		if err != nil {
			return err
		}

		// undo every 3rd block
		if b%3 == 2 {
			err := f.Undo(*ub)
			if err != nil {
				return err
			}
			sc.BackOne(adds, durations, delHashes)
			afterRoot := f.GetRoots()
			if !reflect.DeepEqual(beforeRoot, afterRoot) {
				return fmt.Errorf("undo mismatch")
			}
		}

	}
	return nil
}

func Fuzz(dataBytes []byte) int {
	data := bytes.NewBuffer(dataBytes)

	err := undoOnceFuzzy(data)
	if err != nil {
		panic("failed")
	}
	return 1
}
