package csn

import (
	"fmt"
	"os"
	"time"

	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"github.com/mit-dci/utreexo/cmd/ttl"
	"github.com/mit-dci/utreexo/cmd/util"
	"github.com/mit-dci/utreexo/utreexo"
	"github.com/syndtr/goleveldb/leveldb"
)

// Here we write proofs for all the txs.
// All the inputs are saved as 32byte sha256 hashes.
// All the outputs are saved as LeafTXO type.
func genPollard(
	tx []*btcutil.Tx,
	height int32,
	totalTXOAdded *int,
	lookahead int32,
	totalDels *int,
	plustime time.Duration,
	pFile *os.File,
	pOffsetFile *os.File,
	lvdb *leveldb.DB,
	p *utreexo.Pollard) error {

	plusstart := time.Now()

	blockAdds, err := genAdds(tx, lvdb, height, totalTXOAdded, lookahead)

	donetime := time.Now()
	plustime += donetime.Sub(plusstart)

	// Grab the proof by height
	bpBytes, err := getProof(uint32(height), pFile, pOffsetFile)
	if err != nil {
		return err
	}
	bp, err := utreexo.FromBytesBlockProof(bpBytes)
	if err != nil {
		return err
	}

	// Fills in the nieces for verification/deletion
	err = p.IngestBlockProof(bp)
	if err != nil {
		return err
	}

	// totalTXOAdded += len(blockAdds) // TODO
	*totalDels += len(bp.Targets)

	// Utreexo tree modification. blockAdds are the added txos and
	// bp.Targets are the positions of the leaves to delete
	err = p.Modify(blockAdds, bp.Targets)
	if err != nil {
		return err
	}
	return nil
}

func genAdds(tx []*btcutil.Tx, lvdb *leveldb.DB, height int32,
	totalTXOAdded *int, lookahead int32) (
	blockAdds []utreexo.LeafTXO, err error) {

	blocktxs := []*util.Txotx{new(util.Txotx)}
	var msgTxs []*wire.MsgTx

	// grab all the MsgTx
	for _, tx := range tx {
		msgTxs = append(msgTxs, tx.MsgTx())
	}
	for _, msgTx := range msgTxs {

		// creates all txos up to index indicated
		numoutputs := uint32(len(msgTx.TxOut))

		// Cache the txhash as it's expensive to generate
		txhash := msgTx.TxHash().String()

		// For tagging each TXO as unspenable
		blocktxs[len(blocktxs)-1].Unspendable = make([]bool, numoutputs)
		for i, out := range msgTx.TxOut {
			if util.IsUnspendable(out) {
				// Tag all the unspenables
				blocktxs[len(blocktxs)-1].Unspendable[i] = true
			} else {
				blocktxs[len(blocktxs)-1].Outputtxid = txhash

				blocktxs[len(blocktxs)-1].DeathHeights =
					make([]uint32, numoutputs)
			}
		}

		// done with this txotx, make the next one and append
		blocktxs = append(blocktxs, new(util.Txotx))

	}
	//TODO Get rid of this. This eats up cpu
	//we started a tx but shouldn't have
	blocktxs = blocktxs[:len(blocktxs)-1]
	// call function to make all the db lookups and find deathheights
	ttl.LookupBlock(blocktxs, lvdb)

	for _, blocktx := range blocktxs {
		adds, err := genLeafTXO(blocktx, uint32(height+1))
		if err != nil {
			return nil, err
		}
		for _, a := range adds {

			// Set bool to true to cache and not redownload from server
			a.Remember = a.Duration < lookahead

			*totalTXOAdded++

			blockAdds = append(blockAdds, a)
		}
	}
	return
}

// Gets the proof for a given block height
func getProof(height uint32, pFile *os.File, pOffsetFile *os.File) ([]byte, error) {

	var offset [4]byte
	pOffsetFile.Seek(int64(height*4), 0)
	pOffsetFile.Read(offset[:])
	if offset == [4]byte{} && height != uint32(0) {
		panic(fmt.Errorf("offset returned nil\nIt's likely that genproofs was exited before finishing\nRun genproofs again and that will probably fix the problem"))
	}

	pFile.Seek(int64(util.BtU32(offset[:])), 0)

	var heightbytes [4]byte
	pFile.Read(heightbytes[:])

	var compare0 [4]byte
	copy(compare0[:], heightbytes[:])

	var compare1 [4]byte
	copy(compare1[:], utreexo.U32tB(height+1))
	//check if height matches
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

// genLeafTXO generates a slice of LeafTXOs with the Duration of how long each
// that TXO lasts attached to them. Skips all OP_RETURNs and TXOs that are spent on the
// same block. UTXOs get a Duration of 1 << 30. Which is just an arbitrary big number
// to make sure that it's bigger than the lookahead so they don't get remembered.
func genLeafTXO(tx *util.Txotx, height uint32) ([]utreexo.LeafTXO, error) {
	adds := []utreexo.LeafTXO{}
	for i := 0; i < len(tx.DeathHeights); i++ {
		if tx.Unspendable[i] == true {
			continue
		}
		// Skip all txos that are spent on the same block
		// Does the same thing as DedupeHashSlices()
		if tx.DeathHeights[i]-height == 0 {
			continue
		}
		// if the DeathHeights is 0, it means it's a UTXO. Shouldn't be remembered
		if tx.DeathHeights[i] == 0 {
			utxostring := fmt.Sprintf("%s:%d", tx.Outputtxid, i)
			addData := utreexo.LeafTXO{
				Hash:     utreexo.HashFromString(utxostring),
				Duration: int32(1 << 30)} // arbitrary big number
			adds = append(adds, addData)

		} else {
			// Write the ttl (time to live).
			// The value is just the duration of how many blocks
			// it took for the TXO to be spent
			// ttl is just 'SpentBlockHeight - CreatedBlockHeight'
			utxostring := fmt.Sprintf("%s:%d", tx.Outputtxid, i)
			addData := utreexo.LeafTXO{
				Hash:     utreexo.HashFromString(utxostring),
				Duration: int32(tx.DeathHeights[i] - height)}
			adds = append(adds, addData)
		}
	}
	return adds, nil
}
