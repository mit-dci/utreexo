package simutil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"math/big"
	"os"
)

// CBlockUndo ...
type CBlockUndo struct {
	Magic uint32
	Size  uint32
	Data  []byte
	Txs   []*CTxUndo
	Hash  [32]byte
}

// CTxUndo ...
type CTxUndo struct {
	Coins []*Coin
}

// Coin ...
type Coin struct {
	Height   uint32
	CoinBase int
	Txout    *CTxOut
}

// CTxOut ...
type CTxOut struct {
	Script []byte
	Amount uint64
}

// UndoReadFromReader ...
func UndoReadFromReader(r io.Reader) (*CBlockUndo, error) {
	blockundo := new(CBlockUndo)
	buf := make([]byte, 4)
	_, err := r.Read(buf)
	if err != nil {
		return nil, err
	}
	blockundo.Magic = binary.LittleEndian.Uint32(buf)
	_, err = r.Read(buf)
	if err != nil {
		return nil, err
	}
	blockundo.Size = binary.LittleEndian.Uint32(buf)
	if blockundo.Size == 0 {
		return nil, errors.New("zero size")
	}
	blockundo.Data = make([]byte, blockundo.Size)
	_, err = r.Read(blockundo.Data)
	if err != nil {
		return nil, err
	}
	s := bytes.NewBuffer(blockundo.Data)
	count, err := ReadCompactSize(s)
	if err != nil {
		return nil, err
	}
	for i := uint64(0); i < count; i++ {
		tx := new(CTxUndo)
		err = tx.Unserialize(s)
		if err != nil {
			return nil, err
		}
		blockundo.Txs = append(blockundo.Txs, tx)
	}
	s.Reset()
	blockundo.Hash = [32]byte{}
	_, err = r.Read(blockundo.Hash[:])
	if err != nil {
		return nil, err
	}
	return blockundo, nil
}

// UndoReadFromBytes ...
func UndoReadFromBytes(bs []byte) (*CBlockUndo, error) {
	return UndoReadFromReader(bytes.NewBuffer(bs))
}

// UndosReadFromFile ...
func UndosReadFromFile(path string) ([]*CBlockUndo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	_, err = f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	defer f.Close()
	undos := []*CBlockUndo{}
	for {
		undo, err := UndoReadFromReader(f)
		if err != nil {
			if err.Error() == "zero size" || err == io.EOF {
				break
			}
			return nil, err
		}
		undos = append(undos, undo)
	}
	return undos, nil
}

// VerifyBlockHash ...
// Example blockhash : 6fe28c0ab6f1b372c1a6a246ae63f74f931e8365e15a089c68d6190000000000
func VerifyBlockHash(blockhash []byte, undos []*CBlockUndo) (*CBlockUndo, error) {
	if len(blockhash) != 32 {
		return nil, errors.New("invalid hash size")
	}
	var ret *CBlockUndo
	for _, undo := range undos {
		hash := sha256.Sum256(append(blockhash, undo.Data...))
		hash = sha256.Sum256(hash[:])
		if hash == undo.Hash {
			ret = undo
			break
		}
	}
	return ret, nil
}

// WriteCompactSize ...
// https://github.com/bitcoin/bitcoin/blob/master/src/serialize.h#L270
// https://github.com/bitcoin/bitcoin/blob/master/src/serialize.h#L287
func WriteCompactSize(os io.Writer, nSize uint64) {
	if nSize < 253 {
		os.Write([]byte{byte(nSize)})
	} else if nSize <= math.MaxUint16 {
		bs := []byte{253, 0, 0}
		binary.LittleEndian.PutUint16(bs[1:], uint16(nSize))
		os.Write(bs)
	} else if nSize <= math.MaxUint32 {
		bs := []byte{254, 0, 0, 0, 0}
		binary.LittleEndian.PutUint32(bs[1:], uint32(nSize))
		os.Write(bs)
	} else {
		bs := []byte{255, 0, 0, 0, 0, 0, 0, 0, 0}
		binary.LittleEndian.PutUint64(bs[1:], nSize)
		os.Write(bs)
	}
	return
}

// ReadCompactSize ...
// https://github.com/bitcoin/bitcoin/blob/master/src/serialize.h#L270
// https://github.com/bitcoin/bitcoin/blob/master/src/serialize.h#L312
func ReadCompactSize(is io.Reader) (uint64, error) {
	var buf []byte
	buf = make([]byte, 1)
	_, err := is.Read(buf)
	if err != nil {
		return 0, err
	}
	chSize := buf[0]
	nSizeRet := uint64(0)
	if chSize < 253 {
		nSizeRet = uint64(chSize)
	} else if chSize == 253 {
		buf = make([]byte, 2)
		_, err := is.Read(buf)
		if err != nil {
			return 0, err
		}
		nSizeRet = uint64(binary.LittleEndian.Uint16(buf))
		if nSizeRet < 253 {
			return 0, errors.New("non-canonical ReadCompactSize()")
		}
	} else if chSize == 254 {
		buf = make([]byte, 4)
		_, err := is.Read(buf)
		if err != nil {
			return 0, err
		}
		nSizeRet = uint64(binary.LittleEndian.Uint32(buf))
		if nSizeRet < 0x10000 {
			return 0, errors.New("non-canonical ReadCompactSize()")
		}
	} else {
		buf = make([]byte, 8)
		_, err := is.Read(buf)
		if err != nil {
			return 0, err
		}
		nSizeRet = binary.LittleEndian.Uint64(buf)
		if nSizeRet < 0x100000000 {
			return 0, errors.New("non-canonical ReadCompactSize()")
		}
	}
	// https://github.com/bitcoin/bitcoin/blob/master/src/serialize.h#L26
	// static const unsigned int MAX_SIZE = 0x02000000;
	if nSizeRet > uint64(0x02000000) {
		return 0, errors.New("ReadCompactSize(): size too large")
	}
	return nSizeRet, nil
}

// GetSizeOfVarInt ...
// https://github.com/bitcoin/bitcoin/blob/master/src/serialize.h#L344
// https://github.com/bitcoin/bitcoin/blob/master/src/serialize.h#L389
func GetSizeOfVarInt(n uint64) int {
	nRet := 0
	for {
		nRet++
		if n <= 0x7F {
			break
		}
		n = (n >> 7) - 1
	}
	return nRet
}

// WriteVarInt ...
// https://github.com/bitcoin/bitcoin/blob/master/src/serialize.h#L344
// https://github.com/bitcoin/bitcoin/blob/master/src/serialize.h#L406
func WriteVarInt(os io.Writer, n uint64) {
	if n < 0x7f {
		os.Write([]byte{byte(n)})
		return
	}
	bs := []byte{byte(n & 0x7f)}
	for {
		if n <= 0x7f {
			break
		}
		n = (n >> 7) - 1
		bs = append([]byte{byte((n & 0x7f) | 0x80)}, bs...)
	}
	os.Write(bs)
	return
}

// ReadVarInt ...
// https://github.com/bitcoin/bitcoin/blob/master/src/serialize.h#L344
// https://github.com/bitcoin/bitcoin/blob/master/src/serialize.h#L424
func ReadVarInt(is io.Reader) (uint64, error) {
	n := uint64(0)
	b := make([]byte, 1)
	for {
		_, err := is.Read(b)
		if err != nil {
			return 0, err
		}
		chData := b[0]
		n = (n << 7) | uint64(chData&0x7f)
		if (chData & 0x80) > 0 {
			n++
		} else {
			return n, nil
		}
	}
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
	buf := new(bytes.Buffer)
	WriteVarInt(buf, uint64(size+6))
	return append(buf.Bytes(), script...)
}

// DecompressScript ...
// https://github.com/bitcoin/bitcoin/blob/0.19/src/compressor.h#L25
func DecompressScript(bs []byte) ([]byte, error) {
	buf := new(bytes.Buffer)
	size, err := ReadCompactSize(buf)
	if err != nil {
		return nil, err
	}
	switch size {
	case 0x00:
		script := []byte{0x76, 0xa9, 20}
		script = append(script, bs[1:21]...)
		script = append(script, []byte{0x88, 0xac}...)
		return script, nil
	case 0x01:
		script := []byte{0xa9, 20}
		script = append(script, bs[1:21]...)
		script = append(script, 0x87)
		return script, nil
	case 0x02:
		fallthrough
	case 0x03:
		script := []byte{33, byte(size)}
		script = append(script, bs[1:33]...)
		script = append(script, 0xac)
		return script, nil
	case 0x04:
		fallthrough
	case 0x05:
		script := []byte{65, byte(size)}
		x := new(big.Int).SetBytes(bs[1:33])
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
		return script, nil
	default:
		size -= 6
		script := []byte{}
		script = append(script, bs[1:1+size]...)
		return script, nil
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

// Unserialize ...
// https://github.com/bitcoin/bitcoin/blob/master/src/undo.h#L86
func (undo *CTxUndo) Unserialize(s io.Reader) error {
	count, err := ReadCompactSize(s)
	if err != nil {
		return err
	}
	for i := uint64(0); i < count; i++ {
		coin := new(Coin)
		err = coin.Unserialize(s)
		if err != nil {
			return err
		}
		undo.Coins = append(undo.Coins, coin)
	}
	return nil
}

// Unserialize ...
// https://github.com/bitcoin/bitcoin/blob/master/src/undo.h#L47
func (coin *Coin) Unserialize(s io.Reader) error {
	nCode, err := ReadVarInt(s)
	if err != nil {
		return err
	}
	coin.Height = uint32(nCode / 2)
	coin.CoinBase = int(nCode & 1)
	if coin.Height > 0 {
		// Old versions stored the version number for the last spend of
		// a transaction's outputs. Non-final spends were indicated with
		// height = 0.
		_, err := ReadVarInt(s)
		if err != nil {
			return err
		}
	}
	coin.Txout = new(CTxOut)
	// https://github.com/bitcoin/bitcoin/blob/master/src/compressor.h#L86
	nVal, err := ReadVarInt(s)
	if err != nil {
		return err
	}
	coin.Txout.Amount = DecompressAmount(nVal)
	// https://github.com/bitcoin/bitcoin/blob/master/src/compressor.h#L64
	size, err := ReadVarInt(s)
	if err != nil {
		return err
	}
	if size < 2 { // size : 0x00 , 0x01
		script := make([]byte, 21)
		script[0] = byte(size)
		_, err = s.Read(script[1:])
		if err != nil {
			return err
		}
		coin.Txout.Script = script
	} else if size < 6 { // size : 0x02 , 0x03 , 0x04 , 0x05
		script := make([]byte, 33)
		script[0] = byte(size)
		_, err = s.Read(script[1:])
		if err != nil {
			return err
		}
		coin.Txout.Script = script
	} else {
		script := make([]byte, size-6)
		_, err = s.Read(script)
		if err != nil {
			return err
		}
		buf := new(bytes.Buffer)
		WriteVarInt(buf, size)
		coin.Txout.Script = append(buf.Bytes(), script...)
	}
	return nil
}
