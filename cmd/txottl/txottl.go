package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

func main() {
	fmt.Printf("hi\n")

	err := runTxo()
	if err != nil {
		panic(err)
	}
}

type delUnit struct {
	del    Hash
	height uint32
}

type sortableHashSlice []Hash

func (d sortableHashSlice) Len() int      { return len(d) }
func (d sortableHashSlice) Swap(i, j int) { d[i], d[j] = d[j], d[i] }
func (d sortableHashSlice) Less(i, j int) bool {
	return bytes.Compare(d[i][:], d[j][:]) < 1
}

//type sortableDelunit []delUnit

//func (d sortableDelunit) Len() int      { return len(d) }
//func (d sortableDelunit) Swap(i, j int) { d[i], d[j] = d[j], d[i] }
//func (d sortableDelunit) Less(i, j int) bool {
//	return bytes.Compare(d[i].del[:], d[j].del[:]) < 1
//}

// uint32 to 4 bytes.  Always works.
func U32tB(i uint32) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, i)
	return buf.Bytes()
}

// 4 byte slice to uint32.  Returns ffffffff if something doesn't work.
func BtU32(b []byte) uint32 {
	if len(b) != 4 {
		fmt.Printf("Got %x to BtU32 (%d bytes)\n", b, len(b))
		return 0xffffffff
	}
	var i uint32
	buf := bytes.NewBuffer(b)
	binary.Read(buf, binary.BigEndian, &i)
	return i
}

// 8 bytes to uint64.  returns ffff. if it doesn't work.
// take first bytes if it's too long
func BtU64(b []byte) uint64 {
	if len(b) > 8 {
		//		fmt.Printf("Got %x to BtU64 (%d bytes)\n", b, len(b))
		//		return 0xffffffffffffffff
		b = b[:8]
	}
	var i uint64
	buf := bytes.NewBuffer(b)
	binary.Read(buf, binary.BigEndian, &i)
	return i
}

// uint64 to 8 bytes.  Always works.
func U64tB(i uint64) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, i)
	return buf.Bytes()
}
