package txottl

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

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

//type sortableDelunit []delUnit

//func (d sortableDelunit) Len() int      { return len(d) }
//func (d sortableDelunit) Swap(i, j int) { d[i], d[j] = d[j], d[i] }
//func (d sortableDelunit) Less(i, j int) bool {
//	return bytes.Compare(d[i].del[:], d[j].del[:]) < 1
//}
