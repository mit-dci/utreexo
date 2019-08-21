package utreexo

import (
	"crypto/rand"
	"encoding/binary"
	"os"
	"testing"
)

func TestStoreAndRestore(t *testing.T) {
	p := &Pollard{}
	size := 5000 + rndn(5000)
	t.Logf("size : %d", size)
	for i := uint64(0); i < size; i++ {
		r := rnd()
		p.addOne(r, true)
	}
	dels := make([]uint64, rndn(10))
	t.Logf("dels : %d", len(dels))
	for i := range dels {
		dels[i] = rndn(size)
	}
	p.Modify([]LeafTXO{}, dels)
	path := "./tmp.dat"
	os.Remove(path)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Errorf("%+v", err)
		return
	}
	err = p.store(file)
	file.Close()
	if err != nil {
		t.Errorf("%+v", err)
		return
	}
	t.Logf("%+v", p)
	file, err = os.OpenFile(path, os.O_RDONLY, 0644)
	if err != nil {
		t.Errorf("%+v", err)
		return
	}
	p, err = restore(file)
	file.Close()
	if err != nil {
		t.Errorf("%+v", err)
		return
	}
	t.Logf("%+v", p)
}

func rnd() Hash {
	h := Hash{}
	rand.Read(h[:])
	return h
}

func rndn(m uint64) uint64 {
	r := make([]byte, 8)
	rand.Read(r)
	n := binary.LittleEndian.Uint64(r)
	return n % m
}
