package bridgenode

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/wire"
	"github.com/mit-dci/utreexo/util"
)

func TestBuildOffsetFile(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir) // clean up. Always runs

	// grab the datadir for this system
	// use testnet3
	testnetDataDir := filepath.Join(util.GetBitcoinDataDir(), "testnet3")
	// grab testnet3 hash
	testnetHash, err := util.GenHashForNet(wire.TestNet3)
	if err != nil {
		t.Fatal(err)
	}

	tmpOffsetFile := filepath.Join(tmpDir, "offsetfile")
	tmpLastOffsetHeightFile := filepath.Join(tmpDir, "loffsetfile")

	// build offsetfile
	fmt.Println("creating offestfile...")
	lastOffsetHeight, err := buildOffsetFile(testnetDataDir,
		*testnetHash, tmpOffsetFile, tmpLastOffsetHeightFile)
	if err != nil {
		t.Fatal(err)
	}

	lvdb := OpenIndexFile(testnetDataDir)
	bnrChan := make(chan BlockAndRev, 10)

	fmt.Println("checking the offestfile created...")

	// Start the reader
	go BlockAndRevReader(bnrChan, testnetDataDir, tmpOffsetFile,
		lastOffsetHeight, 1)

	// Check that things in the offsetfile are correct
	// 300,000 blocks is prob enough
	for i := int32(1); i < 300000; i++ { // skip genesis
		bnr := <-bnrChan

		cbIdx := GetBlockIndexInfo(bnr.Blk.BlockHash(), lvdb)
		// Check that the height is correct
		if cbIdx.Height != i {
			err := fmt.Errorf(
				"CBlockFileIndex height is: %d but it should be %d",
				cbIdx.Height, i)
			t.Fatal(err)
		}
		// Check that there are same number of txs and rev txs (minus coinbase)
		if len(bnr.Blk.Transactions)-1 != len(bnr.Rev.Txs) {
			err := fmt.Errorf(
				"block height: %d has %d txs but rev block has: %d txs",
				i, len(bnr.Blk.Transactions), len(bnr.Rev.Txs))
			t.Fatal(err)
		}
		if i%100000 == 0 {
			fmt.Println("# of tested blocks: ", i)
		}
	}
}
