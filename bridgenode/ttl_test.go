package bridgenode

import (
	"bytes"
	"fmt"
	"math/rand"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

func TestSearch(t *testing.T) {

	searchSize := 17

	// make random txs

	miniTxs := make([]miniTx, searchSize)
	var pos int
	for i, _ := range miniTxs {
		h := chainhash.HashH([]byte{byte(i >> 8), byte(i)})
		miniTxs[i].txid = &h
		miniTxs[i].startsAt = uint16(pos)
		pos += rand.Intn(10)
	}
	sortTxids(miniTxs)

	var buf bytes.Buffer
	for _, mt := range miniTxs {
		err := mt.serialize(&buf)
		if err != nil {
			fmt.Printf("miniTx write error: %s\n", err.Error())
		}
	}

	fmt.Printf("%x\n", buf.Bytes())
	reader := bytes.NewReader(buf.Bytes())
	// pick one to search for
	searchMiniIn := miniIn{idx: 0, height: 0}
	copy(searchMiniIn.hashprefix[:], miniTxs[rand.Intn(searchSize)].txid[:6])

	result := binSearch(searchMiniIn, 0, int64(searchSize), reader)
	fmt.Printf("result: %d\n", result)

}
