package simutil

import (
	"crypto/sha256"
)

type Hash [32]byte

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
