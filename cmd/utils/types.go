package simutil

import (
	"crypto/sha256"

	"github.com/mit-dci/lit/wire"
)

type Hash [32]byte

//simutil.Hash is just [32]byte
var MainnetGenHash = Hash{
	0x6f, 0xe2, 0x8c, 0x0a, 0xb6, 0xf1, 0xb3, 0x72,
	0xc1, 0xa6, 0xa2, 0x46, 0xae, 0x63, 0xf7, 0x4f,
	0x93, 0x1e, 0x83, 0x65, 0xe1, 0x5a, 0x08, 0x9c,
	0x68, 0xd6, 0x19, 0x00, 0x00, 0x00, 0x00, 0x00,
}

var TestNet3GenHash = Hash{
	0x43, 0x49, 0x7f, 0xd7, 0xf8, 0x26, 0x95, 0x71,
	0x08, 0xf4, 0xa3, 0x0f, 0xd9, 0xce, 0xc3, 0xae,
	0xba, 0x79, 0x97, 0x20, 0x84, 0xe9, 0x0e, 0xad,
	0x01, 0xea, 0x33, 0x09, 0x00, 0x00, 0x00, 0x00,
}

//HashFromString hahes the given string with sha256
func HashFromString(s string) Hash {
	return sha256.Sum256([]byte(s))
}

//Struct for a tx to be converted to LeafTXOs
type Txotx struct {
	//txid of the tx
	Outputtxid string

	//Whether the output is an OP_RETURN or not
	Unspendable []bool

	//When the output is spent
	DeathHeights []uint32
}

//Header data read off the .dat file.
//FileNum is the .dat file number
//Offset is where it is in the .dat file.
//CurrentHeaderHash is the 80byte header double hashed
//Prevhash is the 32 byte previous header included in the 80byte header.
type RawHeaderData struct {
	CurrentHeaderHash [32]byte
	Prevhash          [32]byte
	FileNum           [4]byte
	Offset            [4]byte
}

type BlockToWrite struct {
	Txs    []*wire.MsgTx
	Height int
}
