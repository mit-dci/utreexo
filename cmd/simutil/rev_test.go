package simutil_test

import (
	"bytes"
	"encoding/hex"
	"math/rand"
	"reflect"
	"testing"

	"github.com/mit-dci/lit/btcutil/txscript"
	"github.com/mit-dci/lit/crypto/koblitz"

	"github.com/mit-dci/utreexo/cmd/simutil"
)

const REV00000DAT_0 = "" +
	"f9beb4d9010000000090c1d8d0ad21e0" +
	"fc9266492be7fdb3d07348a74aa6f165" +
	"5d478589178a3589ad"

const BLOCK_HASH_0 = "000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f"

const REV00000DAT_1 = "" +
	"f9beb4d90100000000f2c2727e535da5" +
	"d314238c5e841b4346af303cecc8356b" +
	"f2b739c8cd786b34a3"

const BLOCK_HASH_1 = "00000000839a8e6886ab5951d76f411475428afc90947ee320161bbf18eb6048"

func TestVerifyBlockHash(t *testing.T) {
	bs, err := hex.DecodeString(REV00000DAT_0 + REV00000DAT_1)
	if err != nil {
		t.Errorf("%+v", err)
		return
	}
	undo, err := simutil.UndoReadFromBytes(bs)
	if err != nil {
		t.Errorf("%+v", err)
		return
	}
	bh, err := hex.DecodeString(BLOCK_HASH_0)
	if err != nil {
		t.Errorf("%+v", err)
		return
	}
	for i, j := 0, len(bh)-1; i < j; i, j = i+1, j-1 {
		bh[i], bh[j] = bh[j], bh[i]
	}
	ret, err := simutil.VerifyBlockHash(bh, []*simutil.CBlockUndo{undo})
	if err != nil {
		t.Errorf("%+v", err)
		return
	}
	if !reflect.DeepEqual(undo, ret) {
		t.Errorf("unmatch undo")
		return
	}
	bs, err = hex.DecodeString(REV00000DAT_0 + REV00000DAT_1)
	if err != nil {
		t.Errorf("%+v", err)
		return
	}
	undo, err = simutil.UndoReadFromBytes(bs[len(REV00000DAT_0)/2:])
	if err != nil {
		t.Errorf("%+v", err)
		return
	}
	bh, err = hex.DecodeString(BLOCK_HASH_1)
	if err != nil {
		t.Errorf("%+v", err)
		return
	}
	for i, j := 0, len(bh)-1; i < j; i, j = i+1, j-1 {
		bh[i], bh[j] = bh[j], bh[i]
	}
	ret, err = simutil.VerifyBlockHash(bh, []*simutil.CBlockUndo{undo})
	if err != nil {
		t.Errorf("%+v", err)
		return
	}
	if !reflect.DeepEqual(undo, ret) {
		t.Errorf("unmatch undo")
		return
	}
}

func TestCompactSize(t *testing.T) {
	// BOOST_AUTO_TEST_CASE(compactsize)
	// https://github.com/bitcoin/bitcoin/blob/master/src/test/serialize_tests.cpp#L231
	MAX_SIZE := uint64(0x02000000)
	ss := new(bytes.Buffer)
	for i := uint64(1); i <= MAX_SIZE; i *= 2 {
		simutil.WriteCompactSize(ss, i-1)
		simutil.WriteCompactSize(ss, i)
	}
	for i := uint64(1); i <= MAX_SIZE; i *= 2 {
		j, err := simutil.ReadCompactSize(ss)
		if err != nil {
			t.Errorf("simutil.ReadCompactSize error : %+v", err)
			return
		}
		if i-1 != j {
			t.Errorf("decoded:%d expected:%d", j, i-1)
			return
		}
		j, err = simutil.ReadCompactSize(ss)
		if err != nil {
			t.Errorf("simutil.ReadCompactSize error : %+v", err)
			return
		}
		if i != j {
			t.Errorf("decoded:%d expected:%d", j, i)
			return
		}
	}
	// BOOST_AUTO_TEST_CASE(noncanonical)
	// https://github.com/bitcoin/bitcoin/blob/master/src/test/serialize_tests.cpp#L270
	ss.Reset()
	var err error
	ss.Write([]byte{0xfd, 0x00, 0x00})
	_, err = simutil.ReadCompactSize(ss)
	if err == nil || err.Error() != "non-canonical ReadCompactSize()" {
		t.Errorf("%+v", err)
	}
	ss.Write([]byte{0xfd, 0xfc, 0x00})
	_, err = simutil.ReadCompactSize(ss)
	if err == nil || err.Error() != "non-canonical ReadCompactSize()" {
		t.Errorf("%+v", err)
	}
	ss.Write([]byte{0xfd, 0xfd, 0x00})
	n, err := simutil.ReadCompactSize(ss)
	if err != nil {
		t.Errorf("%+v", err)
	}
	if n != 0xfd {
		t.Errorf("0xfd")
	}
	ss.Write([]byte{0xfe, 0x00, 0x00, 0x00, 0x00})
	_, err = simutil.ReadCompactSize(ss)
	if err == nil || err.Error() != "non-canonical ReadCompactSize()" {
		t.Errorf("%+v", err)
	}
	ss.Write([]byte{0xfe, 0xff, 0xff, 0x00, 0x00})
	_, err = simutil.ReadCompactSize(ss)
	if err == nil || err.Error() != "non-canonical ReadCompactSize()" {
		t.Errorf("%+v", err)
	}
	ss.Write([]byte{0xfe, 0xff, 0xff, 0x00, 0x00})
	_, err = simutil.ReadCompactSize(ss)
	if err == nil || err.Error() != "non-canonical ReadCompactSize()" {
		t.Errorf("%+v", err)
	}
	ss.Write([]byte{0xff, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	_, err = simutil.ReadCompactSize(ss)
	if err == nil || err.Error() != "non-canonical ReadCompactSize()" {
		t.Errorf("%+v", err)
	}
	ss.Write([]byte{0xff, 0xff, 0xff, 0xff, 0x01, 0x00, 0x00, 0x00, 0x00})
	_, err = simutil.ReadCompactSize(ss)
	if err == nil || err.Error() != "non-canonical ReadCompactSize()" {
		t.Errorf("%+v", err)
	}
}

func TestVarInt(t *testing.T) {
	// BOOST_AUTO_TEST_CASE(varints)
	// https://github.com/bitcoin/bitcoin/blob/master/src/test/serialize_tests.cpp#L178
	// encode
	ss := new(bytes.Buffer)
	size := 0
	for i := uint64(0); i < 100000; i++ {
		simutil.WriteVarInt(ss, i)
		size += simutil.GetSizeOfVarInt(i)
		if size != len(ss.Bytes()) {
			t.Errorf("unmatch size")
			return
		}
	}
	for i := uint64(0); i < 100000000000; i += 999999937 {
		simutil.WriteVarInt(ss, i)
		size += simutil.GetSizeOfVarInt(i)
		if size != len(ss.Bytes()) {
			t.Errorf("unmatch size")
			return
		}
	}
	// decode
	for i := uint64(0); i < 100000; i++ {
		j, err := simutil.ReadVarInt(ss)
		if err != nil {
			t.Errorf("simutil.ReadVarInt error : %+v", err)
			return
		}
		if i != j {
			t.Errorf("decoded:%d expected:%d", j, i)
			return
		}
	}
	for i := uint64(0); i < 100000000000; i += 999999937 {
		j, err := simutil.ReadVarInt(ss)
		if err != nil {
			t.Errorf("simutil.ReadVarInt error : %+v", err)
			return
		}
		if i != j {
			t.Errorf("decoded:%d expected:%d", j, i)
			return
		}
	}
	// BOOST_AUTO_TEST_CASE(varints_bitpatterns)
	// https://github.com/bitcoin/bitcoin/blob/master/src/test/serialize_tests.cpp#L210
	nums := []uint64{0, 0x7f, 0x80, 0x1234, 0xffff, 0x123456, 0x80123456, 0xffffffff, 0x7fffffffffffffff, 0xffffffffffffffff}
	data := [][]byte{
		[]byte{0x00}, []byte{0x7f}, []byte{0x80, 0x00}, []byte{0xa3, 0x34},
		[]byte{0x82, 0xfe, 0x7f}, []byte{0xc7, 0xe7, 0x56}, []byte{0x86, 0xff, 0xc7, 0xe7, 0x56},
		[]byte{0x8e, 0xfe, 0xfe, 0xfe, 0x7f}, []byte{0xfe, 0xfe, 0xfe, 0xfe, 0xfe, 0xfe, 0xfe, 0xfe, 0x7f},
		[]byte{0x80, 0xfe, 0xfe, 0xfe, 0xfe, 0xfe, 0xfe, 0xfe, 0xfe, 0x7f},
	}
	for i, _ := range nums {
		ss := new(bytes.Buffer)
		simutil.WriteVarInt(ss, nums[i])
		bs := ss.Bytes()
		n, err := simutil.ReadVarInt(bytes.NewBuffer(data[i]))
		if err != nil {
			t.Errorf("simutil.ReadVarInt error : %+v", err)
			return
		}
		if (n != nums[i]) || (!reflect.DeepEqual(bs, data[i])) {
			t.Errorf("expected: %d %d %x", i, nums[i], data[i])
			t.Errorf("decoded : %d %x %d", i, bs, n)
		}
		ss.Reset()
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
