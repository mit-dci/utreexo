package util

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"

	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

// Hash is just [32]byte
var mainNetGenHash = Hash{
	0x6f, 0xe2, 0x8c, 0x0a, 0xb6, 0xf1, 0xb3, 0x72,
	0xc1, 0xa6, 0xa2, 0x46, 0xae, 0x63, 0xf7, 0x4f,
	0x93, 0x1e, 0x83, 0x65, 0xe1, 0x5a, 0x08, 0x9c,
	0x68, 0xd6, 0x19, 0x00, 0x00, 0x00, 0x00, 0x00,
}

var testNet3GenHash = Hash{
	0x43, 0x49, 0x7f, 0xd7, 0xf8, 0x26, 0x95, 0x71,
	0x08, 0xf4, 0xa3, 0x0f, 0xd9, 0xce, 0xc3, 0xae,
	0xba, 0x79, 0x97, 0x20, 0x84, 0xe9, 0x0e, 0xad,
	0x01, 0xea, 0x33, 0x09, 0x00, 0x00, 0x00, 0x00,
}

var regTestGenHash = Hash{
	0x06, 0x22, 0x6e, 0x46, 0x11, 0x1a, 0x0b, 0x59,
	0xca, 0xaf, 0x12, 0x60, 0x43, 0xeb, 0x5b, 0xbf,
	0x28, 0xc3, 0x4f, 0x3a, 0x5e, 0x33, 0x2a, 0x1f,
	0xc7, 0xb2, 0xb7, 0x3c, 0xf1, 0x88, 0x91, 0x0f,
}

// For a given BitcoinNet, yields the genesis hash
// If the BitcoinNet is not supported, an error is
// returned.
func GenHashForNet(net wire.BitcoinNet) (*Hash, error) {
	switch net {
	case wire.TestNet3:
		return &testNet3GenHash, nil
	case wire.MainNet:
		return &mainNetGenHash, nil
	case wire.TestNet: // yes, this is regtest
		return &regTestGenHash, nil
	}
	return nil, fmt.Errorf("net not supported\n")
}

// Checks if the blk00000.dat file in the current directory
// is testnet3 or mainnet or regtest.
func CheckNet(net wire.BitcoinNet) {
	f, err := os.Open("blk00000.dat")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	var magicbytes [4]byte
	f.Read(magicbytes[:])

	bytesToMatch := U32tLB(uint32(net))

	if bytes.Compare(magicbytes[:], bytesToMatch) != 0 {
		switch net {
		case wire.TestNet3:
			fmt.Println("Option -net=testnet given but .dat file is NOT a testnet file.")
		case wire.MainNet:
			fmt.Println("Neither option -net=testnet or -net=regtest was given but .dat file is NOT a mainnet file.")
		case wire.TestNet:
			fmt.Println("Option -net=regtest given but .dat file is NOT a regtest file.")
		}
		fmt.Println("Exiting...")
		os.Exit(2)
	}
}

// BlockReader is a wrapper around GetRawBlockFromFile so that the process
// can be made into a goroutine. As long as it's running, it keeps sending
// the entire blocktxs and height to bchan with BlockToWrite type.
func BlockReader(
	txChan chan TxToWrite, currentOffsetHeight, height int32, offsetfile string) {
	for height != currentOffsetHeight {
		txs, err := GetRawBlockFromFile(height, offsetfile)
		if err != nil {
			panic(err)
		}
		send := TxToWrite{Txs: txs, Height: height}
		txChan <- send
		height++
	}
}

// GetRawBlocksFromFile reads the blocks from the given .dat file and
// returns those blocks.
// Skips the genesis block. If you search for block 0, it will give you
// block 1.
func GetRawBlockFromFile(tipnum int32, offsetFileName string) (
	txs []*btcutil.Tx, err error) {

	var datFile [4]byte
	var offset [4]byte

	offsetFile, err := os.Open(offsetFileName)
	if err != nil {
		return nil, err
	}

	// offset file consists of 8 bytes per block
	// tipnum * 8 gives us the correct position for that block
	offsetFile.Seek(int64(8*tipnum), 0)

	// Read file and offset for the block
	offsetFile.Read(datFile[:])
	offsetFile.Read(offset[:])

	fileName := fmt.Sprintf("blk%05d.dat", int(BtU32(datFile[:])))
	// Channel to alert stopParse() that offset
	f, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}
	// +8 skips the 8 bytes of magicbytes and load size
	f.Seek(int64(BtU32(offset[:])+8), 0)

	// TODO this is probably expensive. fix
	b := new(wire.MsgBlock)
	err = b.Deserialize(f)
	if err != nil {
		return nil, err
	}
	f.Close()
	offsetFile.Close()

	for _, msgTx := range b.Transactions {
		txs = append(txs, btcutil.NewTx(msgTx))
	}

	return
}

// U32tB converts uint32 to 4 bytes.  Always works.
func U32tB(i uint32) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, i)
	return buf.Bytes()
}

// TODO make actual error return here
// 4 byte Big Endian slice to uint32.  Returns ffffffff if something doesn't work.
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

// int32 to 4 bytes (Big Endian).  Always works.
func I32tB(i int32) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, i)
	return buf.Bytes()
}

// TODO make actual error return here
// 4 byte Big Endian slice to uint32.  Returns ffffffff if something doesn't work.
func BtI32(b []byte) int32 {
	if len(b) != 4 {
		fmt.Printf("Got %x to ItU32 (%d bytes)\n", b, len(b))
		return -0x7fffffff
	}
	var i int32
	buf := bytes.NewBuffer(b)
	binary.Read(buf, binary.BigEndian, &i)
	return i
}

// uint32 to 4 bytes (Little Endian).  Always works.
func U32tLB(i uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, i)
	return b
}

// Converts 4 byte Little Endian slices to uint32.
// Returns ffffffff if something doesn't work.
func LBtU32(b []byte) uint32 {
	if len(b) != 4 {
		fmt.Printf("Got %x to LBtU32 (%d bytes)\n", b, len(b))
		return 0xffffffff
	}
	var i uint32
	buf := bytes.NewBuffer(b)
	binary.Read(buf, binary.LittleEndian, &i)
	return i
}

// BtU64 : 8 bytes to uint64.  returns ffff. if it doesn't work.
func BtU64(b []byte) uint64 {
	if len(b) != 8 {
		fmt.Printf("Got %x to BtU64 (%d bytes)\n", b, len(b))
		return 0xffffffffffffffff
	}
	var i uint64
	buf := bytes.NewBuffer(b)
	binary.Read(buf, binary.BigEndian, &i)
	return i
}

// U64tB : uint64 to 8 bytes.  Always works.
func U64tB(i uint64) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, i)
	return buf.Bytes()
}

// CheckMagicByte checks for the Bitcoin magic bytes.
// returns false if it didn't read the Bitcoin magic bytes.
// Checks only for testnet3 and mainnet
func CheckMagicByte(bytesgiven [4]byte) bool {
	if bytesgiven != [4]byte{0x0b, 0x11, 0x09, 0x07} && //testnet
		bytesgiven != [4]byte{0xf9, 0xbe, 0xb4, 0xd9} { // mainnet
		fmt.Printf("got non magic bytes %x, finishing\n", bytesgiven)
		return false
	} else {
		return true
	}
}

// HasAccess reports whether we have access to the named file.
// Returns true if HasAccess, false if it doesn't.
// Does NOT tell us if the file exists or not.
// File might exist but may not be available to us
func HasAccess(fileName string) bool {
	if _, err := os.Stat(fileName); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

//IsUnspendable determines whether a tx is spenable or not.
//returns true if spendable, false if unspenable.
func IsUnspendable(o *wire.TxOut) bool {
	switch {
	case len(o.PkScript) > 10000: //len 0 is OK, spendable
		return true
	case len(o.PkScript) > 0 && o.PkScript[0] == 0x6a: //OP_RETURN is 0x6a
		return true
	default:
		return false
	}
}
