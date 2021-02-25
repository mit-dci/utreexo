package main

import (
	"bytes"
	"fmt"
	"os"

	"github.com/mit-dci/utreexo/bridgenode"
	"github.com/mit-dci/utreexo/btcacc"
)

// a cBlock or clairvoyant block is all the data needed for the clairvoyant
// schedule generator for a particular block height
type cBlock struct {
	blockHeight int32
	ttls        []int32 // addHashes[i] corresponds with ttls[i]; same length
}

// NOTE I think we don't actually need to keep track of insertions or deletions
// at all, and ONLY the TTLs are needed!
// Because, who cares *what* the element being added is, the only reason to
// be able to identify it is so we can find it to remove it.  But we
// can assign it a sequential number instead of using a hash

func main() {
	fmt.Printf("reclair file reader")

	// this initializes the configuration of files and directories to be read
	cfg, err := bridgenode.Parse(os.Args[1:])
	if err != nil {
		panic(err)
	}

	cBlocks, err := getCBlocks(1, 10, *cfg)
	if err != nil {
		panic(err)
	}
	fmt.Printf("got %d cblocks\n", len(cBlocks))
}

func getCBlocks(start, count int32, cfg bridgenode.Config) ([]cBlock, error) {
	// build cblock slice to return
	cblocks := make([]cBlock, count)

	// grab utreexo data and populate cblocks
	for i, _ := range cblocks {
		udataBytes, err := bridgenode.GetUDataBytesFromFile(
			cfg.UtreeDir.ProofDir, start+int32(i))
		if err != nil {
			return nil, err
		}
		udbuf := bytes.NewBuffer(udataBytes)
		var udata btcacc.UData
		udata.Deserialize(udbuf)

		// put together the cblock
		// height & ttls we can get right away in the format we need from udata
		cblocks[i].blockHeight = udata.Height
		cblocks[i].ttls = udata.TxoTTLs
	}
	return cblocks, nil
}
