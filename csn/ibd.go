package csn

import (
	"fmt"
	"time"

	"github.com/btcsuite/btcd/wire"
	"github.com/mit-dci/utreexo/accumulator"
	"github.com/mit-dci/utreexo/util"
)

// run IBD from block proof data
// we get the new utxo info from the same txos text file
func (c *Csn) IBDThread(sig chan bool) {

	// Channel to alert the main loop to break when receiving a quit signal from
	// the OS
	haltRequest := make(chan bool, 1)

	// Channel to alert stopRunIBD it's ok to exit
	// Makes it wait for flushing to disk
	haltAccept := make(chan bool, 1)

	go stopRunIBD(sig, haltRequest, haltAccept)

	// caching parameter. Keeps txos that are spent before than this many blocks
	lookahead := int32(1000)

	// for benchmarking
	var totalTXOAdded, totalDels int

	// blocks come in and sit in the blockQueue
	// They should come in from the network -- right now they're coming from the
	// disk but it should be the exact same thing
	ublockQueue := make(chan util.UBlock, 10)

	// Reads blocks asynchronously from blk*.dat files, and the proof.dat, and DB
	// this will be a network reader, with the server sending the same stuff over
	go util.UblockNetworkReader(
		ublockQueue, "127.0.0.1:8338", c.CurrentHeight, lookahead)

	var plustime time.Duration
	starttime := time.Now()

	// bool for stopping the below for loop
	var stop bool
	for ; !stop; c.CurrentHeight++ {
		blocknproof, open := <-ublockQueue
		if !open {
			sig <- true
			break
		}

		err := putBlockInPollard(blocknproof,
			&totalTXOAdded, &totalDels, plustime, &c.pollard, c.CheckSignatures)
		if err != nil {
			panic(err)
		}
		c.HeightChan <- c.CurrentHeight

		c.ScanBlock(blocknproof.Block)

		if c.CurrentHeight%10000 == 0 {
			fmt.Printf("Block %d add %d del %d %s plus %.2f total %.2f \n",
				c.CurrentHeight, totalTXOAdded, totalDels, c.pollard.Stats(),
				plustime.Seconds(), time.Now().Sub(starttime).Seconds())
		}

		// Check if stopSig is no longer false
		// stop = true makes the loop exit
		select {
		case stop = <-haltRequest:
		default:
		}

	}
	fmt.Printf("Block %d add %d del %d %s plus %.2f total %.2f \n",
		c.CurrentHeight, totalTXOAdded, totalDels, c.pollard.Stats(),
		plustime.Seconds(), time.Now().Sub(starttime).Seconds())

	saveIBDsimData(c.CurrentHeight, c.pollard)

	fmt.Println("Done Writing")

	haltAccept <- true
}

// ScanBlock looks through a block using the CSN's maps and sends matches
// into the tx channel.
func (c *Csn) ScanBlock(b wire.MsgBlock) {
	var curAdr [20]byte
	for _, tx := range b.Transactions {
		// first check utxo loss
		for _, in := range tx.TxIn {
			lostTxo, exists := c.utxoStore[in.PreviousOutPoint]
			if !exists {
				continue
			}
			delete(c.utxoStore, in.PreviousOutPoint)
			c.totalScore -= lostTxo.Amt
			fmt.Printf("tx %s lost %d satoshis :( But still have %d in %d utxos\n",
				tx.TxHash().String(), lostTxo.Amt, c.totalScore, len(c.utxoStore))
			c.TxChan <- *tx
		}

		// now check utxo gain
		for i, out := range tx.TxOut {
			if len(out.PkScript) != 22 {
				continue
			}
			copy(curAdr[:], out.PkScript[2:])
			if c.WatchAdrs[curAdr] {
				newOut := wire.OutPoint{Hash: tx.TxHash(), Index: uint32(i)}
				c.RegisterOutPoint(newOut)
				c.utxoStore[newOut] =
					util.LeafData{Outpoint: newOut, Amt: out.Value}
				c.totalScore += out.Value
				fmt.Printf("got utxo %s with %d satoshis! Now have %d in %d utxos\n",
					newOut.String(), out.Value, c.totalScore, len(c.utxoStore))
				c.TxChan <- *tx
				// break
			}
		}
	}
}

// Here we write proofs for all the txs.
// All the inputs are saved as 32byte sha256 hashes.
// All the outputs are saved as LeafTXO type.
func putBlockInPollard(
	ub util.UBlock,
	totalTXOAdded, totalDels *int,
	plustime time.Duration,
	p *accumulator.Pollard,
	checkSig bool) error {

	plusstart := time.Now()

	inskip, outskip := util.DedupeBlock(&ub.Block)
	if !ub.ProofsProveBlock(inskip) {
		return fmt.Errorf("uData missing utxo data for block %d", ub.Height)
	}

	*totalDels += len(ub.ExtraData.AccProof.Targets) // for benchmarking

	// derive leafHashes from leafData
	if !ub.ExtraData.Verify(p.ReconstructStats()) {
		return fmt.Errorf("height %d LeafData / Proof mismatch", ub.Height)
	}

	// **************************************
	// check transactions and signatures here
	// TODO: it'd be better to do it after IngestBatchProof(),
	// or really in the middle of IngestBatchProof(), after it does
	// verifyBatchProof(), but before it actually starts populating / modifying
	// the pollard.  This is because verifying the proof should be faster than
	// checking all the signatures in the block, so we'd rather do the fast
	// thing first.  (Especially since that thing isn't committed to in the
	// PoW, but the signatures are...

	if checkSig {
		if !ub.CheckBlock(outskip) {
			return fmt.Errorf("height %d hash %s block invalid",
				ub.Height, ub.Block.BlockHash().String())
		}
	}

	// sort before ingestion; verify up above unsorts...
	ub.ExtraData.AccProof.SortTargets()
	// Fills in the empty(nil) nieces for verification && deletion
	err := p.IngestBatchProof(ub.ExtraData.AccProof)
	if err != nil {
		fmt.Printf("height %d ingest error\n", ub.Height)
		return err
	}

	// get hashes to add into the accumulator
	blockAdds := util.BlockToAddLeaves(
		ub.Block, nil, outskip, ub.Height)
	*totalTXOAdded += len(blockAdds) // for benchmarking

	// fmt.Printf("h %d adds %d targets %d\n",
	// ub.Height, len(blockAdds), len(ub.ExtraData.AccProof.Targets))

	// Utreexo tree modification. blockAdds are the added txos and
	// bp.Targets are the positions of the leaves to delete
	err = p.Modify(blockAdds, ub.ExtraData.AccProof.Targets)
	if err != nil {
		return err
	}

	donetime := time.Now()
	plustime += donetime.Sub(plusstart)

	return nil
}
