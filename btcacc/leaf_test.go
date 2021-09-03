package btcacc

import (
	"bytes"
	"fmt"
	"testing"
)

func TestLeafDataSerialize(t *testing.T) {
	ld := LeafData{
		TxHash:   Hash{1, 2, 3, 4},
		Index:    0,
		Height:   2,
		Coinbase: false,
		Amt:      3000,
		PkScript: []byte{1, 2, 3, 4, 5, 6},
	}

	// Before
	writer := &bytes.Buffer{}
	ld.Serialize(writer)
	beforeBytes := writer.Bytes()

	// After
	checkLeaf := LeafData{}
	checkLeaf.Deserialize(writer)

	afterWriter := &bytes.Buffer{}
	checkLeaf.Serialize(afterWriter)
	afterBytes := afterWriter.Bytes()

	if !bytes.Equal(beforeBytes, afterBytes) {
		err := fmt.Errorf("Serialize/Deserialize LeafData fail\n"+
			"beforeBytes len: %v\n, afterBytes len:%v\n",
			len(beforeBytes), len(afterBytes))
		t.Fatal(err)
	}
}
