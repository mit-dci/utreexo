package simutil

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
	"math/big"
	"os"
)

// RevReader ...
type RevReader struct {
	Records []*BlockRecord
}

// BlockRecord ...
type BlockRecord struct {
	Magic uint32
	Size  uint32
	Data  []byte
	Block *CBlockUndo
	Hash  []byte
}

// CBlockUndo ...
type CBlockUndo struct {
	Txs []*CTxUndo
}

// CTxUndo ...
type CTxUndo struct {
	Ins []*CTxInUndo
}

// CTxInUndo ...
type CTxInUndo struct {
	Height uint64
	Script []byte
	Amount uint64
}

// OpenRevReader ...
func OpenRevReader(name string) (*RevReader, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	r := new(RevReader)
	err = r.init(f, fi.Size())
	if err != nil {
		f.Close()
		return nil, err
	}
	return r, nil
}

// NewRevReader ...
func NewRevReader(r io.ReaderAt, size int64) (*RevReader, error) {
	if size < 0 {
		return nil, errors.New("rev: size cannot be negative")
	}
	rr := new(RevReader)
	err := rr.init(r, size)
	if err != nil {
		return nil, err
	}
	return rr, nil
}

func (rr *RevReader) init(r io.ReaderAt, size int64) error {
	pos := int64(0)
	for size > pos {
		record := new(BlockRecord)
		buf := make([]byte, 4)
		_, err := r.ReadAt(buf, pos)
		if err != nil {
			return err
		}
		record.Magic = binary.LittleEndian.Uint32(buf)
		if record.Magic == 0 {
			break
		}
		pos += 4
		_, err = r.ReadAt(buf, pos)
		if err != nil {
			return err
		}
		record.Size = binary.LittleEndian.Uint32(buf)
		pos += 4
		data := make([]byte, record.Size)
		_, err = r.ReadAt(data, pos)
		if err != nil {
			return err
		}
		record.Data = data
		record.Block = block(data)
		pos += int64(record.Size)
		hash := make([]byte, 32)
		_, err = r.ReadAt(hash, pos)
		if err != nil {
			return err
		}
		record.Hash = hash
		pos += 32
		rr.Records = append(rr.Records, record)
	}
	return nil
}

func block(data []byte) *CBlockUndo {
	block := new(CBlockUndo)
	dp, tsize := BsToCompactSize(data, 0)
	for i := uint64(0); i < tsize; i++ {
		tx := new(CTxUndo)
		ilen, isize := BsToCompactSize(data, dp)
		dp += ilen
		for j := uint64(0); j < isize; j++ {
			in := new(CTxInUndo)
			hlen, height := BsToVarInt(data, dp)
			in.Height = height
			dp += hlen
			if in.Height/2 > 0 { // TODO need?
				dp++
			}
			alen, amt := BsToVarInt(data, dp)
			in.Amount = DecompressAmount(amt)
			dp += alen
			fb := int64(data[dp])
			if fb == 0x00 || fb == 0x01 {
				in.Script = data[dp : dp+21]
				dp += 21
			} else if fb < 0x06 {
				in.Script = data[dp : dp+32]
				dp += 33
			} else {
				slen, ssize := BsToVarInt(data, dp)
				in.Script = data[dp : dp+slen+int64(ssize)-6]
				dp += slen + int64(ssize) - 6
			}
			tx.Ins = append(tx.Ins, in)
		}
		block.Txs = append(block.Txs, tx)
	}
	return block
}

// BsToCompactSize ...
// https://github.com/bitcoin/bitcoin/blob/v0.14.2/src/serialize.h#L202
func BsToCompactSize(bs []byte, start int64) (int64, uint64) {
	size := int64(1)
	n := uint64(bs[start])
	if n == 0xfd {
		n = uint64(binary.LittleEndian.Uint16(bs[start+1 : start+3]))
		size = 3
	} else if n == 0xfe {
		n = uint64(binary.LittleEndian.Uint32(bs[start+1 : start+5]))
		size = 5
	} else if n == 0x0ff {
		n = binary.LittleEndian.Uint64(bs[start+1 : start+9])
		size = 9
	}
	return size, n
}

// CompactSizeToBs ...
// https://github.com/bitcoin/bitcoin/blob/v0.14.2/src/serialize.h#L202
func CompactSizeToBs(n uint64) []byte {
	var bs []byte
	if n < 0xfd {
		bs = []byte{byte(n)}
	} else if n <= math.MaxUint16 {
		bs = []byte{0xfd, 0x00, 0x00}
		binary.LittleEndian.PutUint16(bs[1:], uint16(n))
	} else if n <= math.MaxUint32 {
		bs = []byte{0xfe, 0x00, 0x00, 0x00, 0x00}
		binary.LittleEndian.PutUint32(bs[1:], uint32(n))
	} else {
		bs = []byte{0xff, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
		binary.LittleEndian.PutUint64(bs[1:], n)
	}
	return bs
}

// BsToVarInt ...
// https://github.com/bitcoin/bitcoin/blob/v0.14.2/src/serialize.h#L277
func BsToVarInt(bs []byte, start int64) (int64, uint64) {
	n := uint64(0)
	pos := start
	for {
		b := bs[pos]
		n = (n << 7) | uint64(b&0x7f)
		pos++
		if (b & 0x80) > 0 {
			n++
		} else {
			break
		}
	}
	return pos - start, n
}

// VarIntToBs ...
// https://github.com/bitcoin/bitcoin/blob/v0.14.2/src/serialize.h#L277
func VarIntToBs(n uint64) []byte {
	if n < 0x7f {
		return []byte{byte(n)}
	}
	bs := []byte{byte(n & 0x7f)}
	for {
		if n <= 0x7f {
			break
		}
		n = (n >> 7) - 1
		bs = append([]byte{byte((n & 0x7f) | 0x80)}, bs...)
	}
	return bs
}

// CompressScript ...
// https://github.com/bitcoin/bitcoin/blob/0.19/src/compressor.h#L25
func CompressScript(script []byte) []byte {
	size := len(script)
	if size == 25 && script[0] == 0x76 && script[1] == 0xa9 && script[2] == 20 && script[23] == 0x88 && script[24] == 0xac {
		// p2pkh
		cscript := []byte{0x00}
		cscript = append(cscript, script[3:23]...)
		return cscript
	} else if size == 23 && script[0] == 0xa9 && script[1] == 20 && script[22] == 0x87 {
		// p2sh
		cscript := []byte{0x01}
		cscript = append(cscript, script[2:22]...)
		return cscript
	} else if size == 35 && script[0] == 33 && script[34] == 0xac && (script[1] == 0x02 || script[1] == 0x03) {
		// p2pk(compress point)
		cscript := []byte{script[1]}
		cscript = append(cscript, script[2:34]...)
		return cscript
	} else if size == 67 && script[0] == 65 && script[66] == 0xac && script[1] == 0x04 {
		// p2pk(full point)
		cscript := []byte{0x04 | (script[64] & 0x01)}
		cscript = append(cscript, script[2:34]...)
		return cscript
	}
	cscript := VarIntToBs(uint64(size + 6))
	cscript = append(cscript, script...)
	return cscript
}

// DecompressScript ...
// https://github.com/bitcoin/bitcoin/blob/0.19/src/compressor.h#L25
func DecompressScript(bs []byte, start int64) (int64, []byte) {
	pos, size := BsToVarInt(bs, start)
	switch size {
	case 0x00:
		script := []byte{0x76, 0xa9, 20}
		script = append(script, bs[start+pos:start+pos+20]...)
		script = append(script, []byte{0x88, 0xac}...)
		return pos + 20, script
	case 0x01:
		script := []byte{0xa9, 20}
		script = append(script, bs[start+pos:start+pos+20]...)
		script = append(script, 0x87)
		return pos + 20, script
	case 0x02:
		fallthrough
	case 0x03:
		script := []byte{33, byte(size)}
		script = append(script, bs[start+pos:start+pos+32]...)
		script = append(script, 0xac)
		return pos + 32, script
	case 0x04:
		fallthrough
	case 0x05:
		script := []byte{65, byte(size)}
		x := new(big.Int).SetBytes(bs[start+pos : start+pos+32])
		p, _ := new(big.Int).SetString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEFFFFFC2F", 16)
		y := new(big.Int).ModSqrt(new(big.Int).Mod(new(big.Int).Add(new(big.Int).Exp(x, big.NewInt(3), p), big.NewInt(7)), p), p)
		if uint(size&0x01) != y.Bit(0) {
			y = new(big.Int).Sub(p, y)
		}
		xbs := x.Bytes()
		for len(xbs) < 32 {
			xbs = append([]byte{0x00}, xbs...)
		}
		script = append(script, xbs...)
		ybs := y.Bytes()
		for len(ybs) < 32 {
			ybs = append([]byte{0x00}, ybs...)
		}
		script = append(script, ybs...)
		script = append(script, 0xac)
		return pos + 32, script
	default:
		size -= 6
		script := []byte{}
		script = append(script, bs[start+pos:start+pos+int64(size)]...)
		return pos + int64(size), script
	}
}

// CompressAmount ...
// https://github.com/bitcoin/bitcoin/blob/0.19/src/compressor.cpp#L141
func CompressAmount(n uint64) uint64 {
	if n == 0 {
		return 0
	}
	e := uint64(0)
	for ((n % 10) == 0) && e < 9 {
		n /= 10
		e++
	}
	if e < 9 {
		d := n % 10
		n /= 10
		return 1 + (n*9+d-1)*10 + e
	}
	return 1 + (n-1)*10 + 9
}

// DecompressAmount ...
// https://github.com/bitcoin/bitcoin/blob/0.19/src/compressor.cpp#L141
func DecompressAmount(x uint64) uint64 {
	// x = 0  OR  x = 1+10*(9*n + d - 1) + e  OR  x = 1+10*(n - 1) + 9
	if x == 0 {
		return 0
	}
	x--
	// x = 10*(9*n + d - 1) + e
	e := x % 10
	x /= 10
	n := uint64(0)
	if e < 9 {
		// x = 9*n + d - 1
		d := (x % 9) + 1
		x /= 9
		// x = n
		n = x*10 + d
	} else {
		n = x + 1
	}
	for e > 0 {
		n *= 10
		e--
	}
	return n
}
