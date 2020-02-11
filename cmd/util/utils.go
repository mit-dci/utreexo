package util

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"

	"github.com/mit-dci/lit/wire"
)

// Hash is just [32]byte
var mainnetGenHash = Hash{
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

// Checks if the blk00000.dat file in the current directory
// is testnet3 or mainnet.
func CheckTestnet(isTestnet bool) {
	if isTestnet == true {
		f, err := os.Open("blk00000.dat")
		if err != nil {
			panic(err)
		}
		var magicbytes [4]byte
		f.Read(magicbytes[:])

		// Check if the magicbytes are for testnet
		if magicbytes != [4]byte{0x0b, 0x11, 0x09, 0x07} {
			fmt.Println("Option -testnet=true given but .dat file is NOT a testnet file.")
			fmt.Println("Exiting...")
			os.Exit(2)
		}
		f.Close()
	} else {
		f, err := os.Open("blk00000.dat")
		if err != nil {
			panic(err)
		}
		var magicbytes [4]byte
		f.Read(magicbytes[:])

		// Check if the magicbytes are for mainnet
		if magicbytes != [4]byte{0xf9, 0xbe, 0xb4, 0xd9} {
			fmt.Println("Option -testnet=true not given but .dat file is a testnet file.")
			fmt.Println("Exiting...")
			os.Exit(2)
		}
		f.Close()
	}
}

// BlockReader is a wrapper around GetRawBlockFromFile so that the process
// can be made into a goroutine. As long as it's running, it keeps sending
// the entire blocktxs and height to bchan with BlockToWrite type.
func BlockReader(
	bchan chan BlockToWrite, currentOffsetHeight, height int32, offsetfile string) {
	for height != currentOffsetHeight {
		block, err := GetRawBlockFromFile(height, offsetfile)
		if err != nil {
			panic(err)
		}
		b := BlockToWrite{Txs: block, Height: height}
		bchan <- b
		height++
	}
}

// GetRawBlocksFromFile reads the blocks from the given .dat file and
// returns those blocks.
// Skips the genesis block. If you search for block 0, it will give you
// block 1.
func GetRawBlockFromFile(tipnum int32, offsetFileName string) ([]*wire.MsgTx, error) {
	var datFile [4]byte
	var offset [4]byte

	offsetFile, err := os.Open(offsetFileName)
	if err != nil {
		panic(err)
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
		panic(err)
	}
	// +8 skips the 8 bytes of magicbytes and load size
	f.Seek(int64(BtU32(offset[:])+8), 0)

	// TODO this is probably expensive. fix
	b := new(wire.MsgBlock)
	err = b.Deserialize(f)
	if err != nil {
		panic(err)
	}
	f.Close()
	offsetFile.Close()

	return b.Transactions, nil
}

// uint32 to 4 bytes.  Always works.
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

// uint32 to 4 bytes.  Always works.
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

// HasAccess reports whether we have acces to the named file.
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
