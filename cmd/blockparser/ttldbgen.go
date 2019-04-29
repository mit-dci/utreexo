package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"sync"

	blake2b "github.com/minio/blake2b-simd"
	"github.com/mit-dci/lit/btcutil/chaincfg/chainhash"
	"github.com/mit-dci/lit/wire"
	"github.com/syndtr/goleveldb/leveldb"
)

/* first pass, just add the deletions to the DB.
then go another pass, and just check the adds, making new lines
of their ttls */
// call writeBlock when you have a block in the correct order to write to
// the file / db
func writeBlock(b wire.MsgBlock, height int, f *os.File,
	batchan chan *leveldb.Batch, wg *sync.WaitGroup) error {

	var s string
	blockBatch := new(leveldb.Batch)

	for blockindex, tx := range b.Transactions {
		for _, in := range tx.TxIn {
			if blockindex > 0 { // skip coinbase "spend"
				opString := in.PreviousOutPoint.String()
				s += "-" + opString + "\n"
				h := HashFromString(opString)
				blockBatch.Put(h[:], U32tB(uint32(height)))
			}
		}

		// creates all txos up to index indicated
		s += "+" + wire.OutPoint{tx.TxHash(), uint32(len(tx.TxOut))}.String()

		for i, out := range tx.TxOut {
			if IsUnspendable(out) {
				s += "z" + fmt.Sprintf("%d", i)
			}
		}
		s += "\n"
	}

	//	fmt.Printf("--- sending off %d dels at height %d\n", batch.Len(), height)
	wg.Add(1)
	batchan <- blockBatch

	s += fmt.Sprintf("h: %d\n", height)
	_, err := f.WriteString(s)

	return err
}

// dbWorker writes everything to the db.  It's it's own goroutine so it
// can work at the same time that the reads are happening
func dbWorker(
	bChan chan *leveldb.Batch, lvdb *leveldb.DB, wg *sync.WaitGroup) {

	for {
		b := <-bChan

		fmt.Printf("--- writing batch %d dels\n", b.Len())

		err := lvdb.Write(b, nil)
		if err != nil {
			fmt.Println(err.Error())
		}
		fmt.Printf("wrote %d deletions to leveldb\n", b.Len())
		wg.Done()
	}
}
func HashFromString(s string) chainhash.Hash {
	return blake2b.Sum256([]byte(s))
}

// uint32 to 4 bytes.  Always works.
func U32tB(i uint32) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, i)
	return buf.Bytes()
}

// 4 byte slice to uint32.  Returns ffffffff if something doesn't work.
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
