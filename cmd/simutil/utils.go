package simutil

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/mit-dci/lit/wire"
)

//Hash is just [32]byte
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

//Checks if the blk00000.dat file in the current directory
//is testnet or mainnet
func CheckTestnet(isTestnet bool) {
	if isTestnet == true {
		f, err := os.Open("blk00000.dat")
		if err != nil {
			panic(err)
		}
		var magicbytes [4]byte
		f.Read(magicbytes[:])

		//Check if the magicbytes are for testnet
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

		//Check if the magicbytes are for mainnet
		if magicbytes != [4]byte{0xf9, 0xbe, 0xb4, 0xd9} {
			fmt.Println("Option -testnet=true not given but .dat file is a testnet file.")
			fmt.Println("Exiting...")
			os.Exit(2)
		}
		f.Close()
	}
}

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

//Converts 4 byte Little Endian slices to uint32.
//Returns ffffffff if something doesn't work.
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

//checkMagicByte checks for the Bitcoin magic bytes.
//returns false if it didn't read the Bitcoin magic bytes.
func CheckMagicByte(bytesgiven [4]byte) bool {
	if bytesgiven != [4]byte{0x0b, 0x11, 0x09, 0x07} && //testnet
		bytesgiven != [4]byte{0xf9, 0xbe, 0xb4, 0xd9} { // mainnet
		fmt.Printf("got non magic bytes %x, finishing\n", bytesgiven)
		return false
	} else {
		return true
	}
}

//Reverses the given string.
//"asdf" becomes "fdsa".
func Reverse(s string) (result string) {
	for _, v := range s {
		result = string(v) + result
	}
	return
}

//HasAccess reports whether we have acces to the named file.
//Returns true if HasAccess, false if it doesn't.
//Does NOT tell us if the file exists or not.
//File might exist but may not be available to us
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

//Gets the latest tipnum from the .txos file
func GetTipNum(txos string) (int, error) {
	//check if there is access to the .txos file
	if HasAccess(txos) == false {
		fmt.Println("No .txos file found, Syncing from the genesis block...")
		return 0, nil
	}

	f, err := os.Open(txos)
	if err != nil {
		panic(err)
	}

	//check if the .txos file is empty
	fstat, _ := f.Stat()
	if fstat.Size() == 0 || fstat.Size() == 1 {
		fmt.Println(".txos file is empty. Syncing from the genesis block...")
		return 0, nil
	}

	var all []byte
	buf := make([]byte, 1)
	var x int64
	var s string

	//Reads backwards and appends the character read to `all` until we hit the character "h".
	//probably an empty/corrupted file if we loop more than 20 times
	for s != "h" && x > -20 {
		f.Seek(x, 2)
		f.Read(buf)
		s = fmt.Sprintf("%s", buf)
		//Don't append any of these ascii characters
		if s != "\n" && s != " " && s != "" && s != "\x00" && s != ":" && s != "h" {
			all = append(all, buf...)
		}
		x--
	}

	//return error if we loop more than 20 times. Normally "h" should be found soon
	err1 := errors.New("Couldn't find the tipnum in .txos file")
	if x <= -20 {
		return 0, err1
	}
	tipstring := fmt.Sprintf("%s", all)
	num, err := strconv.Atoi(Reverse(tipstring))
	if err != nil {
		return 0, err
	}
	f.Close()
	return num, nil
}

func GetPOffsetNum(pOffsetCurrentIndexFile *os.File) (uint32, error) {
	var pOffsetByte [4]byte
	_, err := pOffsetCurrentIndexFile.ReadAt(pOffsetByte[:], 0)
	if err != nil {
		return 0, err
	}
	return BtU32(pOffsetByte[:]), nil
}
