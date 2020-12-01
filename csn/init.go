package csn

import (
	"fmt"
	"os"
	"runtime/pprof"
	"runtime/trace"
	"time"

	"github.com/adiabat/bech32"

	"github.com/btcsuite/btcd/wire"
	"github.com/mit-dci/utreexo/accumulator"
	"github.com/mit-dci/utreexo/btcacc"
	"github.com/mit-dci/utreexo/util"
)

// RunIBD calls everything to run IBD
func RunIBD(cfg *Config, sig chan bool) error {
	// Profiling
	if cfg.CpuProf != "" {
		f, err := os.Create(*cpuProfCmd)
		if err != nil {
			return err
		}
		pprof.StartCPUProfile(f)
	}
	if cfg.MemProf != "" {
		f, err := os.Create(*memProfCmd)
		if err != nil {
			return err
		}
		pprof.WriteHeapProfile(f)
	}
	if cfg.TraceProf != "" {
		f, err := os.Create(*traceCmd)
		if err != nil {
			return err
		}
		trace.Start(f)
	}

	// check on disk for pre-existing state and load it
	pol, height, utxos, err := initCSNState()
	if err != nil {
		return err
	}

	pol.Lookahead = int32(cfg.lookAhead)

	// make a new CSN struct and load the pollard into it
	c := Csn{
		pollard:         pol,
		CheckSignatures: cfg.checkSig,
		utxoStore:       utxos,
	}

	txChan, heightChan, err := c.Start(cfg, height, "compactstate", "", sig)
	if err != nil {
		return err
	}

	var pkh [20]byte
	if cfg.watchAddr != "" {
		fmt.Printf("decode len %d %s\n", len(cfg.watchAddr), *watchAddr)
		adrBytes, err := bech32.SegWitAddressDecode(cfg.watchAddr)
		if err != nil {
			return err
		}
		if len(adrBytes) != 22 {
			return fmt.Errorf("need a bech32 p2wpkh address, %s has %d bytes",
				*watchAddr, len(adrBytes))
		}

		copy(pkh[:], adrBytes[2:])
		c.RegisterAddress(pkh)
	}

	for {
		select {
		case tx := <-txChan:
			fmt.Printf("wallet got tx %s\n", tx.TxHash().String())
		case height := <-heightChan:
			if height%1000 == 0 {
				fmt.Printf("got to height %d\n", height)
			}
		}
	}
}

// Start starts up a compact state node, and returns channels for txs and
// block heights.
func (c *Csn) Start(cfg *Config, height int32, path, proxyURL string, haltSig chan bool) (
	chan wire.MsgTx, chan int32, error) {

	// initialize maps
	c.WatchAdrs = make(map[[20]byte]bool)
	c.WatchOPs = make(map[wire.OutPoint]bool)
	//c.utxoStore = make(map[wire.OutPoint]util.LeafData)
	for _, utxo := range c.utxoStore {
		c.totalScore += utxo.Amt
	}

	// initialize channels
	c.TxChan = make(chan wire.MsgTx, 10)
	c.HeightChan = make(chan int32, 10)

	c.CurrentHeight = height
	c.Params = cfg.params
	c.remoteHost = cfg.remoteHost

	// start client & connect
	go c.IBDThread(haltSig, cfg.quitafter)

	return c.TxChan, c.HeightChan, nil
}

// initCSNState attempts to load and initialize the CSN state from the disk.
// If a CSN state is not present, chain is initialized to the genesis
func initCSNState() (
	p accumulator.Pollard, height int32, utxos map[wire.OutPoint]btcacc.LeafData, err error) {

	// bool to check if the pollarddata is present
	pollardInitialized := util.HasAccess(PollardFilePath)

	if pollardInitialized {
		fmt.Println("Has access to forestdata, resuming")
		height, p, utxos, err = restorePollard()
		if err != nil {
			return
		}
	} else {
		fmt.Println("Creating new pollarddata")
		// start at height 1
		height = 1
		utxos = make(map[wire.OutPoint]btcacc.LeafData)
		// Create file needed for pollard
		_, err = os.OpenFile(PollardFilePath, os.O_CREATE, 0600)
		if err != nil {
			return
		}
	}

	return
}

func stopRunIBD(sig chan bool, stopGoing chan bool, done chan bool) {
	// Listen for SIGINT, SIGTERM, and SIGQUIT from the user
	<-sig
	pprof.StopCPUProfile()
	trace.Stop()

	// Sometimes there are bugs that make the program run forever.
	// Utreexo binary should never take more than 10 seconds to exit
	go func() {
		time.Sleep(10 * time.Second)
		fmt.Println("Program timed out. Force quitting." +
			"Data likely corrupted")
		os.Exit(1)
	}()

	// Tell the user that the sig is received
	fmt.Println("User exit signal received. Exiting...")

	// Tell Runibd() to finish the block it's working on
	stopGoing <- true

	// Wait until RunIBD() says it's ok to quit
	<-done
	os.Exit(0)
}
