package blockparser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/mit-dci/lit/btcutil/chaincfg/chainhash"
	"github.com/syndtr/goleveldb/leveldb"
)

// dbWorker writes everything to the db. It's it's own goroutine so it
// can work at the same time that the reads are happening
func dbWorker(
	bChan chan *leveldb.Batch, lvdb *leveldb.DB, wg *sync.WaitGroup) {

	for {
		b := <-bChan
		//		fmt.Printf("--- writing batch %d dels\n", b.Len())
		err := lvdb.Write(b, nil)
		if err != nil {
			fmt.Println(err.Error())
		}
		//		fmt.Printf("wrote %d deletions to leveldb\n", b.Len())
		wg.Done()
	}
}
func HashFromString(s string) chainhash.Hash {
	return sha256.Sum256([]byte(s))
}

// uint32 to 4 bytes.  Always works.
func U32tB(i uint32) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, i)
	return buf.Bytes()
}

//TODO make actual error return here
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
