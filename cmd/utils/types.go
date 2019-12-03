package simutil

import (
	"crypto/sha256"
)

type Hash [32]byte

//HashFromString hahes the given string with sha256
func HashFromString(s string) Hash {
	return sha256.Sum256([]byte(s))
}
