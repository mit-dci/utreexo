package btcacc

import (
	"bytes"
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"

	"github.com/mit-dci/utreexo/common"
)

const HashSize = 32

type Hash [HashSize]byte

// String returns the Hash as the hexadecimal string of the byte-reversed
// hash.
func (hash Hash) String() string {
	for i := 0; i < HashSize/2; i++ {
		hash[i], hash[HashSize-1-i] = hash[HashSize-1-i], hash[i]
	}
	return hex.EncodeToString(hash[:])
}

// LeafData is all the data that goes into a leaf in the utreexo accumulator. Everything here is enough data to verify the bitcoin signatures
type LeafData struct {
	BlockHash [32]byte
	TxHash    Hash
	Index     uint32 // txout index
	Height    int32
	Coinbase  bool
	Amt       int64
	PkScript  []byte
}

// ToString turns a LeafData into a string
func (l *LeafData) ToString() (s string) {
	s = l.OPString()
	// s += fmt.Sprintf(" bh %x ", l.BlockHash)
	s += fmt.Sprintf(" h %d ", l.Height)
	s += fmt.Sprintf("cb %v ", l.Coinbase)
	s += fmt.Sprintf("amt %d ", l.Amt)
	s += fmt.Sprintf("pks %x ", l.PkScript)
	s += fmt.Sprintf("%x ", l.LeafHash())
	s += fmt.Sprintf("size %d", l.SerializeSize())
	return
}

// OPString returns just the outpoint of this leafdata as a string
func (ld *LeafData) OPString() string {
	// Allocate enough for hash string, colon, and 10 digits.  Although
	// at the time of writing, the number of digits can be no greater than
	// the length of the decimal representation of maxTxOutPerMessage, the
	// maximum message payload may increase in the future and this
	// optimization may go unnoticed, so allocate space for 10 decimal
	// digits, which will fit any uint32.
	buf := make([]byte, 2*HashSize+1, 2*HashSize+1+10)
	copy(buf, ld.TxHash.String())
	buf[2*HashSize] = ':'
	buf = strconv.AppendUint(buf, uint64(ld.Index), 10)
	return string(buf)
}

// Serialize puts LeafData onto a writer
func (l *LeafData) Serialize(w io.Writer) (err error) {
	hcb := l.Height << 1
	if l.Coinbase {
		hcb |= 1
	}

	_, err = w.Write(l.BlockHash[:])
	_, err = w.Write(l.TxHash[:])

	freeBytes := common.NewFreeBytes()
	defer freeBytes.Free()

	err = freeBytes.PutUint32(w, binary.BigEndian, l.Index)
	err = freeBytes.PutUint32(w, binary.BigEndian, uint32(hcb))
	err = freeBytes.PutUint64(w, binary.BigEndian, uint64(l.Amt))

	if len(l.PkScript) > 10000 {
		err = fmt.Errorf("pksize too long")
		return
	}
	err = freeBytes.PutUint16(w, binary.BigEndian, uint16(len(l.PkScript)))
	_, err = w.Write(l.PkScript)
	return
}

// SerializeSize says how big a leafdata is
func (l *LeafData) SerializeSize() int {
	// 32B blockhash, 36B outpoint, 4B h/coinbase, 8B amt, 2B pkslen, pks
	// so 82B + pks
	return 82 + len(l.PkScript)
}

func (l *LeafData) Deserialize(r io.Reader) (err error) {
	_, err = io.ReadFull(r, l.BlockHash[:])
	_, err = io.ReadFull(r, l.TxHash[:])
	err = binary.Read(r, binary.BigEndian, &l.Index)
	err = binary.Read(r, binary.BigEndian, &l.Height)
	err = binary.Read(r, binary.BigEndian, &l.Amt)

	var pkSize uint16
	err = binary.Read(r, binary.BigEndian, &pkSize)
	if pkSize > 10000 {
		err = fmt.Errorf("bh %x op %s pksize %d byte too long",
			l.BlockHash, l.OPString(), pkSize)
		return
	}
	l.PkScript = make([]byte, pkSize)
	_, err = io.ReadFull(r, l.PkScript)
	if l.Height&1 == 1 {
		l.Coinbase = true
	}
	l.Height >>= 1
	return
}

// compact serialization for LeafData:
// don't need to send BlockHash; figure it out from height
// don't need to send outpoint, it's already in the msgBlock
// can use tags for PkScript
// so it's just height, coinbaseness, amt, pkscript tag

// LeafHash turns a LeafData into a LeafHash
func (l *LeafData) LeafHash() [32]byte {
	freeBytes := common.NewFreeBytes()
	defer freeBytes.Free()
	buf := bytes.NewBuffer(freeBytes.Bytes)
	l.Serialize(buf)
	return sha512.Sum512_256(buf.Bytes())
}
