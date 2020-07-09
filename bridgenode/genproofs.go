package bridgenode

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/peer"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/wire"
	"github.com/mit-dci/utreexo/accumulator"
	"github.com/mit-dci/utreexo/util"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// build the bridge node / proofs
func BuildProofs(param chaincfg.Params, dataDir string,
	forestInRam, forestCached bool, sig chan bool) error {

	// Channel to alert the tell the main loop it's ok to exit
	haltRequest := make(chan bool)

	// Waitgroup to alert stopBuildProofs() that revoffet and offset has
	// been finished
	offsetFinished := make(chan bool, 1)

	// Channel for stopBuildProofs() to wait
	haltAccept := make(chan bool, 1)

	// Handle user interruptions
	go stopBuildProofs(sig, offsetFinished, haltRequest, haltAccept)

	// Creates all the directories needed for bridgenode
	util.MakePaths()

	// Init forest and variables. Resumes if the data directory exists
	forest, height, offsetFile, err :=
		initBridgeNodeState(param, dataDir, forestInRam, forestCached, offsetFinished)
	if err != nil {
		fmt.Printf("initialization error.  If your .blk and .dat files are ")
		fmt.Printf("not in %s, specify alternate path with -datadir\n.", dataDir)
		return err
	}

	// TODO: Should this channel be buffered?
	newBlock := make(chan bool, 100)
	// TODO: make the peer address a cli option?
	blockAnnouncer, err := connectNode("localhost:8333", param, peer.MessageListeners{
		OnInv: func(p *peer.Peer, msg *wire.MsgInv) {
			for _, invVec := range msg.InvList {
				// If there is a block inv message in the inv list,
				// announce a new block to the offsetfile builder.
				if invVec.Type == wire.InvTypeBlock {
					foreBitcoindCacheFlush(&rpcclient.ConnConfig{
						Host:         "localhost:18332",
						User:         "utreexo",
						Pass:         "utreexo",
						HTTPPostMode: true,
						DisableTLS:   true,
					})
					fmt.Println("New block announcement!")
					newBlock <- true
					break
				}
			}
		},
	})
	if err != nil {
		return err
	}
	defer blockAnnouncer.Disconnect()
	fmt.Println("Listening for block updates from local node")

	lastKnownHeight := offsetFile.Build(newBlock, haltRequest, haltAccept)
	// create offsets for any new blocks that have been created since the last run.
	newBlock <- true

	ttlpath := "utree/" + param.Name + "ttldb"
	// Open leveldb
	o := opt.Options{CompactionTableSizeMultiplier: 8}
	lvdb, err := leveldb.OpenFile(ttlpath, &o)
	if err != nil {
		fmt.Printf("initialization error.  If your .blk and .dat files are ")
		fmt.Printf("not in %s, specify alternate path with -datadir\n.", dataDir)
		return err
	}
	defer lvdb.Close()

	// For ttl value writing
	var batchwg sync.WaitGroup
	batchan := make(chan *leveldb.Batch, 10)

	// Start 16 workers. Just an arbitrary number
	for j := 0; j < 16; j++ {
		go DbWorker(batchan, lvdb, &batchwg)
	}

	// To send/receive blocks from blockreader()
	blockAndRevReadQueue := make(chan BlockAndRev, 10)

	// Reads block asynchronously from .dat files
	// Reads util the lastIndexOffsetHeight
	go BlockAndRevReader(blockAndRevReadQueue, dataDir, "",
		lastKnownHeight, height)
	proofChan := make(chan []byte, 10)
	newProofCondition := sync.NewCond(&sync.Mutex{})
	var fileWait sync.WaitGroup
	go proofWriterWorker(proofChan, newProofCondition, &fileWait)

	fmt.Println("Building Proofs and ttldb...")

	// Add blocks to the forest asynchronously
	go forestWorker(forest, height, forestInRam,
		&batchwg, &fileWait,
		batchan, proofChan, haltAccept, haltRequest, blockAndRevReadQueue)

	// Start the block server
	blockServer(dataDir, haltRequest, haltAccept, newProofCondition)

	wait := make(chan bool)
	<-wait
	return nil
}

func forestWorker(forest *accumulator.Forest, currentForestHeight int32,
	forestInRam bool,
	batchwg *sync.WaitGroup, fileWait *sync.WaitGroup,
	batchan chan *leveldb.Batch,
	proofChan chan []byte,
	haltAccept chan bool,
	haltRequest chan bool,
	blockAndRevReadQueue chan BlockAndRev) error {
	var stop bool

	for !stop {
		bnr := BlockAndRev{}
		// block until a halt is requested or a new BlockAndRev is available
		select {
		case stop = <-haltRequest:
			continue
		case bnr = <-blockAndRevReadQueue:
		}

		// Receive txs from the asynchronous blk*.dat reader
		// loop will block here if there are no blocks in the queue.

		// Writes the ttl values for each tx to leveldb
		WriteBlock(bnr, batchan, batchwg)

		// Get the add and remove data needed from the block & undo block
		blockAdds, delLeaves, err := blockToAddDel(bnr)
		if err != nil {
			return err
		}

		// use the accumulator to get inclusion proofs, and produce a block
		// proof with all data needed to verify the block
		ud, err := genUData(delLeaves, forest, bnr.Height)
		if err != nil {
			return err
		}

		// convert UData struct to bytes
		b := ud.ToBytes()

		// Add to WaitGroup and send data to channel to be written
		// to disk
		fileWait.Add(1)
		proofChan <- b

		ud.AccProof.SortTargets()

		// fmt.Printf("h %d adds %d targets %d\n",
		// 	height, len(blockAdds), len(ud.AccProof.Targets))

		// TODO: Don't ignore undoblock
		// Modifies the forest with the given TXINs and TXOUTs
		_, err = forest.Modify(blockAdds, ud.AccProof.Targets)
		if err != nil {
			return err
		}

		if bnr.Height%100 == 0 {
			fmt.Println("On block :", bnr.Height+1)
		}

		currentForestHeight++
	}

	// wait until dbWorker() has written to the ttldb file
	// allows leveldb to close gracefully
	batchwg.Wait()

	// Wait for the file workers to finish
	fileWait.Wait()

	// Save the current state so genproofs can be resumed
	err := saveBridgeNodeData(forest, currentForestHeight, forestInRam)
	if err != nil {
		panic(err)
	}

	fmt.Println("Saved bridgenode data at height", currentForestHeight)

	if stop {
		// genproofs was paused.
		// Tell stopBuildProofs that it's ok to exit
		haltAccept <- true
		return nil
	}

	return nil
}

// foreBitcoindCacheFlush forces bitcoind to flush all leveldb data to disk by
// usng the scantxoutset rpc call.
// This is a bit hacky since the rpc is not ment for this
// but we need a cache flush and gettxoutsetinfo takes too long.
func foreBitcoindCacheFlush(config *rpcclient.ConnConfig) {
	// TODO: do we really need 2 rpc clients? it would not let me do it with just one
	// even with RawRequestAsync.
	rpcClient1, err := rpcclient.New(config, nil)
	if err != nil {
		panic(err)
	}
	defer rpcClient1.Shutdown()

	rpcClient2, err := rpcclient.New(config, nil)
	if err != nil {
		panic(err)
	}
	defer rpcClient2.Shutdown()

	startJSON, _ := json.Marshal("start")
	abortJSON, _ := json.Marshal("abort")
	argsJSON, _ := json.Marshal([]int{})
	startFuture := rpcClient1.RawRequestAsync("scantxoutset",
		[]json.RawMessage{startJSON, argsJSON})
	rpcClient2.RawRequest("scantxoutset",
		[]json.RawMessage{abortJSON})

	startFuture.Receive()
	fmt.Println("bitcoind cache flushed")
}

// genBlockProof calls forest.ProveBatch with the hash data to get a batched
// inclusion proof from the accumulator. It then adds on the utxo leaf data,
// to create a block proof which both proves inclusion and gives all utxo data
// needed for transaction verification.
func genUData(delLeaves []util.LeafData, f *accumulator.Forest, height int32) (
	ud util.UData, err error) {

	ud.UtxoData = delLeaves
	// make slice of hashes from leafdata
	delHashes := make([]accumulator.Hash, len(ud.UtxoData))
	for i, _ := range ud.UtxoData {
		delHashes[i] = ud.UtxoData[i].LeafHash()
		// fmt.Printf("del %s -> %x\n",
		// ud.UtxoData[i].Outpoint.String(), delHashes[i][:4])
	}
	// generate block proof. Errors if the tx cannot be proven
	// Should never error out with genproofs as it takes
	// blk*.dat files which have already been vetted by Bitcoin Core
	ud.AccProof, err = f.ProveBatch(delHashes)
	if err != nil {
		err = fmt.Errorf("genUData failed at block %d %s %s",
			height, f.Stats(), err.Error())
		return
	}

	if len(ud.AccProof.Targets) != len(delLeaves) {
		err = fmt.Errorf("genUData %d targets but %d leafData",
			len(ud.AccProof.Targets), len(delLeaves))
		return
	}

	// fmt.Printf(batchProof.ToString())
	// Optional Sanity check. Should never fail.

	// unsort := make([]uint64, len(ud.AccProof.Targets))
	// copy(unsort, ud.AccProof.Targets)
	// ud.AccProof.SortTargets()
	// ok := f.VerifyBatchProof(ud.AccProof)
	// if !ok {
	// 	return ud, fmt.Errorf("VerifyBatchProof failed at block %d", height)
	// }
	// ud.AccProof.Targets = unsort

	// also optional, no reason to do this other than bug checking

	// if !ud.Verify(f.ReconstructStats()) {
	// 	err = fmt.Errorf("height %d LeafData / Proof mismatch", height)
	// 	return
	// }
	return
}

func connectNode(addr string, param chaincfg.Params, listeners peer.MessageListeners) (*peer.Peer, error) {
	conn, err := net.Dial("tcp4", addr)
	if err != nil {
		return nil, fmt.Errorf("bitcoind could no be reached at %s: %v", addr, err)
	}

	conf := peer.Config{
		ChainParams: &param,
		Listeners:   listeners,
	}

	node, err := peer.NewOutboundPeer(&conf, addr)
	if err != nil {
		return nil, fmt.Errorf("could not create outboud peer: %v", err)
	}

	node.AssociateConnection(conn)

	return node, nil
}

// genAddDel is a wrapper around genAdds and genDels. It calls those both and
// throws out all the same block spends.
// It's a little redundant to give back both delLeaves and delHashes, since the
// latter is just the hash of the former, but if we only return delLeaves we
// end up hashing them twice which could slow things down.
func blockToAddDel(bnr BlockAndRev) (blockAdds []accumulator.Leaf,
	delLeaves []util.LeafData, err error) {

	inskip, outskip := util.DedupeBlock(&bnr.Blk)
	// fmt.Printf("inskip %v outskip %v\n", inskip, outskip)
	delLeaves, err = blockNRevToDelLeaves(bnr, inskip)
	if err != nil {
		return
	}

	// this is bridgenode, so don't need to deal with memorable leaves
	blockAdds = util.BlockToAddLeaves(bnr.Blk, nil, outskip, bnr.Height)

	return
}

// genDels generates txs to be deleted from the Utreexo forest. These are TxIns
func blockNRevToDelLeaves(bnr BlockAndRev, skiplist []uint32) (
	delLeaves []util.LeafData, err error) {

	// make sure same number of txs and rev txs (minus coinbase)
	if len(bnr.Blk.Transactions)-1 != len(bnr.Rev.Txs) {
		err = fmt.Errorf("genDels block %d %d txs but %d rev txs",
			bnr.Height, len(bnr.Blk.Transactions), len(bnr.Rev.Txs))
		return
	}

	var blockInIdx uint32
	for txinblock, tx := range bnr.Blk.Transactions {
		if txinblock == 0 {
			blockInIdx++ // coinbase tx always has 1 input
			continue
		}
		txinblock--
		// make sure there's the same number of txins
		if len(tx.TxIn) != len(bnr.Rev.Txs[txinblock].TxIn) {
			err = fmt.Errorf("genDels block %d tx %d has %d inputs but %d rev entries",
				bnr.Height, txinblock+1,
				len(tx.TxIn), len(bnr.Rev.Txs[txinblock].TxIn))
			return
		}
		// loop through inputs
		for i, txin := range tx.TxIn {
			// check if on skiplist.  If so, don't make leaf
			if len(skiplist) > 0 && skiplist[0] == blockInIdx {
				// fmt.Printf("skip %s\n", txin.PreviousOutPoint.String())
				skiplist = skiplist[1:]
				blockInIdx++
				continue
			}

			// build leaf
			var l util.LeafData

			l.Outpoint = txin.PreviousOutPoint
			l.Height = bnr.Rev.Txs[txinblock].TxIn[i].Height
			l.Coinbase = bnr.Rev.Txs[txinblock].TxIn[i].Coinbase
			// TODO get blockhash from headers here -- empty for now
			// l.BlockHash = getBlockHashByHeight(l.CbHeight >> 1)
			l.Amt = bnr.Rev.Txs[txinblock].TxIn[i].Amount
			l.PkScript = bnr.Rev.Txs[txinblock].TxIn[i].PKScript
			delLeaves = append(delLeaves, l)
			blockInIdx++
		}
	}
	return
}

// stopBuildProofs listens for the signal from the OS and initiates an exit squence
func stopBuildProofs(
	sig, offsetfinished, haltRequest, haltAccept chan bool) {

	// Listen for SIGINT, SIGQUIT, SIGTERM
	<-sig

	// Sometimes there are bugs that make the program run forver.
	// Utreexo binary should never take more than 10 seconds to exit
	go func() {
		time.Sleep(10 * time.Second)
		fmt.Println("Program timed out. Force quitting. Data likely corrupted")
		os.Exit(1)
	}()

	// Tell the user that the sig is received
	fmt.Println("User exit signal received. Exiting...")
	// TODO: a WaitGroup is probably cleaner
	// haltrequest for forestWorker, blockServer and the offsetfile builder
	haltRequest <- true
	haltRequest <- true
	haltRequest <- true

	// Wait until forestWorker, blockServer and the offsetfile builder say it's ok to exit
	<-haltAccept
	<-haltAccept
	<-haltAccept
	os.Exit(0)
}
