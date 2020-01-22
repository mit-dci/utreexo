package simutil_test

import (
	"math"
	"math/rand"
	"reflect"
	"testing"

	"github.com/mit-dci/lit/btcutil/txscript"
	"github.com/mit-dci/lit/crypto/koblitz"

	"github.com/mit-dci/utreexo/cmd/simutil"
)

func TestCompactSize(t *testing.T) {
	// https://github.com/bitcoin/bitcoin/blob/master/src/test/serialize_tests.cpp#L231
	MAX_SIZE := uint64(0x02000000)
	buf := []byte{}
	for i := uint64(1); i <= MAX_SIZE; i *= 2 {
		bs := simutil.CompactSizeToBs(i - 1)
		buf = append(buf, bs...)
		bs = simutil.CompactSizeToBs(i)
		buf = append(buf, bs...)
	}
	pos := int64(0)
	for i := uint64(1); i <= MAX_SIZE; i *= 2 {
		size, j := simutil.BsToCompactSize(buf, pos)
		pos += size
		if j != i-1 {
			t.Errorf("decoded:%d expected:%d", j, i-1)
		}
		size, j = simutil.BsToCompactSize(buf, pos)
		pos += size
		if j != i {
			t.Errorf("decoded:%d expected:%d", j, i)
		}
	}
	// original test case
	nums := []uint64{0, 1, 252, 253, math.MaxUint16, math.MaxUint16 + 1, math.MaxUint32,
		math.MaxUint32 + 1, math.MaxUint64}
	data := [][]byte{
		[]byte{0x00}, []byte{0x01}, []byte{0xfc}, []byte{0xfd, 0xfd, 0x00}, []byte{0xfd, 0xff, 0xff},
		[]byte{0xfe, 0x00, 0x00, 0x01, 0x00}, []byte{0xfe, 0xff, 0xff, 0xff, 0xff},
		[]byte{0xff, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00},
		[]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
	}
	for i, _ := range nums {
		bs := simutil.CompactSizeToBs(nums[i])
		size, num := simutil.BsToCompactSize(data[i], 0)
		if (num != nums[i]) || (!reflect.DeepEqual(bs, data[i])) || (size != int64(len(data[i]))) {
			t.Errorf("expected: %d %d %x", i, nums[i], data[i])
			t.Errorf("decoded : %d %x %d %d", i, bs, size, num)
		}
	}
}

func TestVarInt(t *testing.T) {
	// https://github.com/bitcoin/bitcoin/blob/master/src/serialize.h#L344
	nums := []uint64{0, 1, 127, 128, 255, 256, 16383, 16384, 16511, 65535, uint64(math.Pow(2, 32))}
	data := [][]byte{
		[]byte{0x00}, []byte{0x01}, []byte{0x7f}, []byte{0x80, 0x00}, []byte{0x80, 0x7f},
		[]byte{0x81, 0x00}, []byte{0xfe, 0x7f}, []byte{0xff, 0x00}, []byte{0xff, 0x7f},
		[]byte{0x82, 0xfe, 0x7f}, []byte{0x8e, 0xfe, 0xfe, 0xff, 0x00},
	}
	for i, _ := range nums {
		bs := simutil.VarIntToBs(nums[i])
		size, num := simutil.BsToVarInt(data[i], 0)
		if (num != nums[i]) || (!reflect.DeepEqual(bs, data[i])) || (size != int64(len(data[i]))) {
			t.Errorf("expected: %d %d %x", i, nums[i], data[i])
			t.Errorf("decoded : %d %x %d %d", i, bs, size, num)
		}
	}
	// https://github.com/bitcoin/bitcoin/blob/master/src/test/serialize_tests.cpp#L210
	nums = []uint64{0, 0x7f, 0x80, 0x1234, 0xffff, 0x123456, 0x80123456, 0xffffffff, 0x7fffffffffffffff, 0xffffffffffffffff}
	data = [][]byte{
		[]byte{0x00}, []byte{0x7f}, []byte{0x80, 0x00}, []byte{0xa3, 0x34},
		[]byte{0x82, 0xfe, 0x7f}, []byte{0xc7, 0xe7, 0x56}, []byte{0x86, 0xff, 0xc7, 0xe7, 0x56},
		[]byte{0x8e, 0xfe, 0xfe, 0xfe, 0x7f}, []byte{0xfe, 0xfe, 0xfe, 0xfe, 0xfe, 0xfe, 0xfe, 0xfe, 0x7f},
		[]byte{0x80, 0xfe, 0xfe, 0xfe, 0xfe, 0xfe, 0xfe, 0xfe, 0xfe, 0x7f},
	}
	for i, _ := range nums {
		bs := simutil.VarIntToBs(nums[i])
		size, num := simutil.BsToVarInt(data[i], 0)
		if (num != nums[i]) || (!reflect.DeepEqual(bs, data[i])) || (size != int64(len(data[i]))) {
			t.Errorf("expected: %d %d %x", i, nums[i], data[i])
			t.Errorf("decoded : %d %x %d %d", i, bs, size, num)
		}
	}
}

func TestCompressedScript(t *testing.T) {
	// https://github.com/bitcoin/bitcoin/blob/master/src/test/compress_tests.cpp#L65
	sb := txscript.NewScriptBuilder()
	sb.AddOp(txscript.OP_DUP)
	sb.AddOp(txscript.OP_HASH160)
	pkh := make([]byte, 20)
	_, err := rand.Read(pkh)
	if err != nil {
		t.Errorf("(p2pkh) rand.Read error : %+v", err)
		return
	}
	sb.AddData(pkh)
	sb.AddOp(txscript.OP_EQUALVERIFY)
	sb.AddOp(txscript.OP_CHECKSIG)
	script, _ := sb.Script()
	if len(script) != 25 {
		t.Errorf("(p2pkh) illegal script size")
		return
	}
	out := simutil.CompressScript(script)
	if len(out) != 21 {
		t.Errorf("(p2pkh) illegal compress script size")
		return
	}
	if out[0] != 0x00 {
		t.Errorf("(p2pkh) illegal first byte of compress script")
		return
	}
	if !reflect.DeepEqual(out[1:], script[3:23]) {
		t.Errorf("(p2pkh) illegal bytes of compress script")
		return
	}
	// https://github.com/bitcoin/bitcoin/blob/master/src/test/compress_tests.cpp#L85
	sb.Reset()
	sb.AddOp(txscript.OP_HASH160)
	sh := make([]byte, 20)
	_, err = rand.Read(sh)
	if err != nil {
		t.Errorf("(p2sh) rand.Read error : %+v", err)
		return
	}
	sb.AddData(sh)
	sb.AddOp(txscript.OP_EQUAL)
	script, _ = sb.Script()
	if len(script) != 23 {
		t.Errorf("(p2sh) illegal script size")
		return
	}
	out = simutil.CompressScript(script)
	if len(out) != 21 {
		t.Errorf("(p2sh) illegal compress script size")
		return
	}
	if out[0] != 0x01 {
		t.Errorf("(p2sh) illegal first byte of compress script")
		return
	}
	if !reflect.DeepEqual(out[1:], script[2:22]) {
		t.Errorf("(p2sh) illegal bytes of compress script")
		return
	}
	// https://github.com/bitcoin/bitcoin/blob/master/src/test/compress_tests.cpp#L102
	pk := make([]byte, 32)
	_, err = rand.Read(pk)
	if err != nil {
		t.Errorf("(compressed p2pk) rand.Read error : %+v", err)
		return
	}
	_, pub := koblitz.PrivKeyFromBytes(koblitz.S256(), pk)
	sb.Reset()
	sb.AddData(pub.SerializeCompressed())
	sb.AddOp(txscript.OP_CHECKSIG)
	script, _ = sb.Script()
	if len(script) != 35 {
		t.Errorf("(compressed p2pk) illegal script size")
		return
	}
	out = simutil.CompressScript(script)
	if len(out) != 33 {
		t.Errorf("(compressed p2pk) illegal compress script size")
		return
	}
	if out[0] != script[1] {
		t.Errorf("(compressed p2pk) illegal first byte of compress script")
		return
	}
	if !reflect.DeepEqual(out[1:], script[2:34]) {
		t.Errorf("(compressed p2pk) illegal bytes of compress script")
		return
	}
	// https://github.com/bitcoin/bitcoin/blob/master/src/test/compress_tests.cpp#L120
	pk = make([]byte, 32)
	_, err = rand.Read(pk)
	if err != nil {
		t.Errorf("(p2pk) rand.Read error : %+v", err)
		return
	}
	_, pub = koblitz.PrivKeyFromBytes(koblitz.S256(), pk)
	sb.Reset()
	sb.AddData(pub.SerializeUncompressed())
	sb.AddOp(txscript.OP_CHECKSIG)
	script, _ = sb.Script()
	if len(script) != 67 {
		t.Errorf("(p2pk) illegal script size")
		return
	}
	out = simutil.CompressScript(script)
	if len(out) != 33 {
		t.Errorf("(p2pk) illegal compress script size")
		return
	}
	if !reflect.DeepEqual(out[1:], script[2:34]) {
		t.Errorf("(p2pk) illegal bytes of compress script")
		return
	}
	if out[0] != (0x04 | (script[65] & 0x01)) {
		t.Errorf("(p2pk) illegal first byte of compress script")
		return
	}
}

func TestCompressAmount(t *testing.T) {
	// https://github.com/bitcoin/bitcoin/blob/master/src/test/compress_tests.cpp#L40
	CENT := uint64(1000000)
	COIN := uint64(100000000)
	decs := []uint64{0, 1, CENT, COIN, 50 * COIN, 21000000 * COIN}
	encs := []uint64{0x0, 0x1, 0x7, 0x9, 0x32, 0x1406f40}
	for i, _ := range decs {
		enc := simutil.CompressAmount(decs[i])
		dec := simutil.DecompressAmount(encs[i])
		if (dec != decs[i]) || (enc != encs[i]) {
			t.Errorf("expected: %d %d %x", i, decs[i], encs[i])
			t.Errorf("decoded : %d %x %d", i, dec, enc)
		}
	}
	// amounts 0.00000001 .. 0.00100000
	NUM_MULTIPLES_UNIT := uint64(100000)
	// amounts 0.01 .. 100.00
	NUM_MULTIPLES_CENT := uint64(10000)
	// amounts 1 .. 10000
	NUM_MULTIPLES_1BTC := uint64(10000)
	// amounts 50 .. 21000000
	NUM_MULTIPLES_50BTC := uint64(420000)
	for i := uint64(1); i <= NUM_MULTIPLES_UNIT; i++ {
		in := i
		if in != simutil.DecompressAmount(simutil.CompressAmount(in)) {
			t.Errorf("(unit) illegal CompressAmount -> DecompressAmount : %d", in)
		}
	}
	for i := uint64(1); i <= NUM_MULTIPLES_CENT; i++ {
		in := i * CENT
		if in != simutil.DecompressAmount(simutil.CompressAmount(in)) {
			t.Errorf("(cent) illegal CompressAmount -> DecompressAmount : %d", in)
		}
	}
	for i := uint64(1); i <= NUM_MULTIPLES_1BTC; i++ {
		in := i * COIN
		if in != simutil.DecompressAmount(simutil.CompressAmount(in)) {
			t.Errorf("(1btc) illegal CompressAmount -> DecompressAmount : %d", in)
		}
	}
	for i := uint64(1); i <= NUM_MULTIPLES_50BTC; i++ {
		in := i * 50 * COIN
		if in != simutil.DecompressAmount(simutil.CompressAmount(in)) {
			t.Errorf("(50btc) illegal CompressAmount -> DecompressAmount : %d", in)
		}
	}
	for i := uint64(0); i <= 100000; i++ {
		in := i
		if in != simutil.CompressAmount(simutil.DecompressAmount(in)) {
			t.Errorf("(unit) illegal DecompressAmount -> CompressAmount : %d", in)
		}
	}
}
