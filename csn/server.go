package csn

import (
	"fmt"
	"os"

	"github.com/btcsuite/btcutil"
	"github.com/mit-dci/utreexo/accumulator"
	"github.com/mit-dci/utreexo/util"
	"github.com/mit-dci/utreexo/util/ttl"

	"github.com/syndtr/goleveldb/leveldb"
)

// genAdds generates txos that are turned into LeafTXOs from the given Txs in a block
// so it's ready to be added to the tree
func genAdds(txs []*btcutil.Tx, db *leveldb.DB,
	height int32, lookahead int32) (blockAdds []accumulator.Leaf, err error) {

	// grab all the MsgTx
	for blockIndex, tx := range txs {
		// Cache the txid as it's expensive to generate
		txid := tx.MsgTx().TxHash().String()

		var remaining int // counter for how many txos to receive
		deathChan := make(chan ttl.DeathInfo)
		// Start goroutines to fetch from leveldb
		for txIndex, out := range tx.MsgTx().TxOut {
			if util.IsUnspendable(out) {
				continue
			}
			remaining++
			go dbLookUp(txid, int32(txIndex),
				int32(blockIndex), lookahead, deathChan, db)
		}
		// Receive from the dbLookUp gorountines
		txoWithDeathHeight := make([]ttl.DeathInfo, remaining)
		for remaining > 0 {
			x := <-deathChan
			if x.DeathHeight < 0 {
				panic("negative deathheight")
			}
			// This insures the tx is in order even though it receives
			// out of order
			txoWithDeathHeight[x.TxPos] = x
			remaining--
		}
		// iter through txoWithDeathHeight and append to blockAdds
		for _, txo := range txoWithDeathHeight {
			// Skip same block spends
			// deathHeight is where the tx is spent. Height+1 represents
			// what the current block height is
			if txo.DeathHeight-(height+1) == 0 {
				continue
			}
			// 0 means it's a UTXO. Don't remember it
			if txo.DeathHeight == 0 {
				add := accumulator.Leaf{Hash: txo.Txid}
				blockAdds = append(blockAdds, add)
			} else {
				add := accumulator.Leaf{
					Hash: txo.Txid,
					// Duration: txo.DeathHeight - (height + 1),
					// Only remember if duration is less than the
					// lookahead value
					Remember: txo.DeathHeight-(height+1) < lookahead}
				blockAdds = append(blockAdds, add)
			}
		}
	}
	return blockAdds, nil
}

// getProof gets the proof for a given block height from the flat files
func getBlockProof(height uint32, pFile *os.File, pOffsetFile *os.File) ([]byte, error) {

	// offset is always 4 bytes. Doing height * 4 will give you the 4 bytes of
	// offset information that you want for that block height
	var offset [4]byte
	pOffsetFile.Seek(int64(height*4), 0)
	pOffsetFile.Read(offset[:])

	// height 0 doesn't have offset information. If height isn't 0 and the offset
	// read is nil, offsetdata/ incorrect
	if offset == [4]byte{} && height != uint32(0) {
		panic(fmt.Errorf("offset returned nil\n" +
			"Likely that genproofs was exited before finishing\n" +
			"Run genproofs again and that will likely fix the problem"))
	}

	// Seek to the proof
	pFile.Seek(int64(util.BtU32(offset[:])), 0)

	var heightbytes [4]byte
	pFile.Read(heightbytes[:])

	// This is just for sanity checking. The height read from the file should
	// match the height that was passed as the argument to getProof
	var compare0, compare1 [4]byte
	copy(compare0[:], heightbytes[:])
	copy(compare1[:], util.U32tB(height+1))
	// check if height matches
	if compare0 != compare1 {
		fmt.Println("read:, given:", compare0, compare1)
		return nil, fmt.Errorf("Corrupted proofoffset file\n")
	}

	var proofsize [4]byte
	pFile.Read(proofsize[:])

	proof := make([]byte, int(util.BtU32(proofsize[:])))
	pFile.Read(proof[:])

	return proof, nil
}

// dbLookUp does the hashing and db read, then returns it's result
// via a channel
func dbLookUp(
	txid string,
	txIndex, blockIndex, lookahead int32,
	addChan chan ttl.DeathInfo, db *leveldb.DB) {

	// build string and hash it (nice that this in parallel too)
	utxostring := fmt.Sprintf("%s:%d", txid, txIndex)
	opHash := util.HashFromString(utxostring)

	// make DB lookup
	ttlbytes, err := db.Get(opHash[:], nil)
	if err == leveldb.ErrNotFound {
		ttlbytes = make([]byte, 4) // not found is 0
	} else if err != nil {
		// some other error
		panic(err)
	}
	if len(ttlbytes) != 4 {
		fmt.Printf("val len %d, op %s:%d\n", len(ttlbytes), txid, txIndex)
		panic("ded")
	}

	addChan <- ttl.DeathInfo{DeathHeight: util.BtI32(ttlbytes),
		TxPos: int32(txIndex), Txid: opHash}
	return
}
