package accumulator

import (
	"bytes"
	"testing"
)

func TestPollardSerializeDeserialize(t *testing.T) {
	var p, q Pollard
	// generate slice of leaf
	leaves := make([]Leaf, 10)
	for i := 0; i < len(leaves); i++ {
		leaves[i].Hash[0] = uint8(i + 1)
	}
	// add leaves to pollard
	err := p.add(leaves)
	if err != nil {
		t.Fatal(err)
	}
	// performing serialization
	old_byte, err := p.Serialize()
	if err != nil {
		t.Fatal(err)
	}
	// perform Deserialize
	err = q.Deserialize(old_byte)
	if err != nil {
		t.Fatal(err)
	}
	// Serialize again and compare bytes
	new_byte, err := q.Serialize()
	if err != nil {
		t.Fatal(err)
	}
	res := bytes.Equal(old_byte, new_byte)
	// If comaprison unequal return error
	if !res {
		t.Fatal("Bytes Unequal")
	}
}
