package blockparser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"sync"

	"github.com/mit-dci/lit/btcutil/chaincfg/chainhash"
	"github.com/mit-dci/lit/wire"
	"github.com/syndtr/goleveldb/leveldb"
)
//writeBlock writes to the .txos file.
//Adds - for txinput, - for txoutput, z for unspenable txos, and the height number for that block
func writeBlock(b wire.MsgBlock, tipnum int, f *os.File, cb *os.File,
	batchan chan *leveldb.Batch, wg *sync.WaitGroup) error {

	var s string
	blockBatch := new(leveldb.Batch)

	for blockindex, tx := range b.Transactions {
		for _, in := range tx.TxIn {
			if blockindex > 0 { // skip coinbase "spend"
				opString := in.PreviousOutPoint.String()
				s += "-" + opString + "\n"
				h := HashFromString(opString)
				blockBatch.Put(h[:], U32tB(uint32(tipnum)))
			}
		}

		// creates all txos up to index indicated
		s += "+" + wire.OutPoint{tx.TxHash(), uint32(len(tx.TxOut))}.String()

		for i, out := range tx.TxOut {
			if isUnspendable(out) {
				s += "z" + fmt.Sprintf("%d", i)
			}
		}
		s += "\n"
	}

	//fmt.Printf("--- sending off %d dels at tipnum %d\n", batch.Len(), tipnum)
	wg.Add(1)
	batchan <- blockBatch

	s += fmt.Sprintf("h: %d\n", tipnum)
	_, err := f.WriteString(s)
	if err != nil {
		panic(err)
	}
	cbh := fmt.Sprintf("%d", tipnum)
	cb.WriteAt([]byte(cbh), 0)

	return err
}

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
