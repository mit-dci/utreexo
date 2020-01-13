package ibdsim

import (
	"fmt"
	"os"
	"time"

	"github.com/mit-dci/lit/wire"
	"github.com/mit-dci/utreexo/cmd/simutil"
	"github.com/mit-dci/utreexo/utreexo"
	"github.com/syndtr/goleveldb/leveldb"
)

//Here we write proofs for all the txs.
//All the inputs are saved as 32byte sha256 hashes.
//All the outputs are saved as LeafTXO type.
func genPollard(
	tx []*wire.MsgTx,
	height int,
	totalTXOAdded *int,
	lookahead int32,
	totalDels *int,
	plustime time.Duration,
	pFile *os.File,
	pOffsetFile *os.File,
	lvdb *leveldb.DB,
	p *utreexo.Pollard) error {

	var blockAdds []utreexo.LeafTXO
	blocktxs := []*simutil.Txotx{new(simutil.Txotx)}
	plusstart := time.Now()

	for _, tx := range tx {

		//creates all txos up to index indicated
		txhash := tx.TxHash()
		//fmt.Println(txhash.String())
		numoutputs := uint32(len(tx.TxOut))

		blocktxs[len(blocktxs)-1].Unspendable = make([]bool, numoutputs)
		//Adds z and index for all OP_RETURN transactions
		//We don't keep track of the OP_RETURNS so probably can get rid of this
		for i, out := range tx.TxOut {
			if simutil.IsUnspendable(out) {
				// Skip all the unspendables
				blocktxs[len(blocktxs)-1].Unspendable[i] = true
			} else {
				//txid := tx.TxHash().String()
				blocktxs[len(blocktxs)-1].Outputtxid = txhash.String()
				blocktxs[len(blocktxs)-1].DeathHeights = make([]uint32, numoutputs)
			}
		}

		// done with this txotx, make the next one and append
		blocktxs = append(blocktxs, new(simutil.Txotx))

	}
	//TODO Get rid of this. This eats up cpu
	//we started a tx but shouldn't have
	blocktxs = blocktxs[:len(blocktxs)-1]
	// call function to make all the db lookups and find deathheights
	lookupBlock(blocktxs, lvdb)

	for _, blocktx := range blocktxs {
		adds, err := genLeafTXO(blocktx, uint32(height+1))
		if err != nil {
			return err
		}
		for _, a := range adds {

			if a.Duration == 0 {
				continue
			}
			//fmt.Println("lookahead: ", lookahead)
			a.Remember = a.Duration < lookahead
			//fmt.Println("Remember", a.Remember)

			*totalTXOAdded++

			blockAdds = append(blockAdds, a)
			//fmt.Println("adds:", blockAdds)
		}
	}
	donetime := time.Now()
	plustime += donetime.Sub(plusstart)
	bpBytes, err := getProof(uint32(height), pFile, pOffsetFile)
	if err != nil {
		return err
	}
	bp, err := utreexo.FromBytesBlockProof(bpBytes)
	if err != nil {
		return err
	}
	if len(bp.Targets) > 0 {
		fmt.Printf("block proof for block %d targets: %v\n", height+1, bp.Targets)
	}
	err = p.IngestBlockProof(bp)
	if err != nil {
		return err
	}

	// totalTXOAdded += len(blockAdds)
	*totalDels += len(bp.Targets)

	err = p.Modify(blockAdds, bp.Targets)
	if err != nil {
		return err
	}
	return nil
}

// Gets the proof for a given block height
func getProof(height uint32, pFile *os.File, pOffsetFile *os.File) ([]byte, error) {

	var offset [4]byte
	pOffsetFile.Seek(int64(height*4), 0)
	pOffsetFile.Read(offset[:])

	pFile.Seek(int64(simutil.BtU32(offset[:])), 0)

	var heightbytes [4]byte
	pFile.Read(heightbytes[:])

	var compare0 [4]byte
	copy(compare0[:], heightbytes[:])

	var compare1 [4]byte
	copy(compare1[:], utreexo.U32tB(height))
	//check if height matches
	if compare0 != compare1 {
		return nil, fmt.Errorf("Corrupted proofoffset file\n")
	}

	var proofsize [4]byte
	pFile.Read(proofsize[:])

	proof := make([]byte, int(simutil.BtU32(proofsize[:])))
	pFile.Read(proof[:])

	return proof, nil

}

// plusLine reads in a line of text, generates a utxo leaf, and determines
// if this is a leaf to remember or not.
func genLeafTXO(tx *simutil.Txotx, height uint32) ([]utreexo.LeafTXO, error) {
	//fmt.Println("DeathHeights len:", len(tx.deathHeights))
	adds := []utreexo.LeafTXO{}
	for i := 0; i < len(tx.DeathHeights); i++ {
		if tx.Unspendable[i] == true {
			continue
		}
		utxostring := fmt.Sprintf("%s;%d", tx.Outputtxid, i)
		addData := utreexo.LeafTXO{
			Hash:     utreexo.HashFromString(utxostring),
			Duration: int32(tx.DeathHeights[i] - height)}
		adds = append(adds, addData)
		// fmt.Printf("expire in\t%d remember %v\n", ttlval[i], addData.Remember)
	}
	return adds, nil
}
