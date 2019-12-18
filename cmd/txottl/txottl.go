package txottl

import (
	"bytes"

	"github.com/mit-dci/utreexo/cmd/utils"
)

type delUnit struct {
	del    simutil.Hash
	height uint32
}

type sortableHashSlice []simutil.Hash

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
