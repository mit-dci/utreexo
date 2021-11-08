package bridgenode

import (
	"bytes"
	"fmt"

	"github.com/syndtr/goleveldb/leveldb"
)

// GetBlockIndexInfo returns a CBlockFileIndex based on the hash given as a key
func GetBlockIndexInfo(h [32]byte, lvdb *leveldb.DB) CBlockFileIndex {
	// 0x62 is hex representation of ascii 'b' (98), which is used
	// appended to block keys in leveldb
	lookup := append([]byte{0x62}, h[:]...)

	value, err := lvdb.Get(lookup, nil)
	if err == leveldb.ErrClosed { // Handle db closed err
		panic(err)
	}
	// Sometimes there may be a block in blk that is not verified but is just sitting there
	// Warn the user about it but ignore it since it doesn't effect the actual validation
	if err != nil { // all other returned errors are from reading the db
		str := fmt.Errorf("%s WARNING: A block in blk file exists without"+
			"a corresponding rev block location. May be wasting disk space", err)
		fmt.Println(str)
	}

	r := bytes.NewReader(value)
	cbIdx := ReadCBlockFileIndex(r)

	return cbIdx
}
