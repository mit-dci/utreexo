package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/btcsuite/btcd/chaincfg"

	bridge "github.com/mit-dci/utreexo/bridgenode"
	"github.com/mit-dci/utreexo/util"
)

var msg = `
Usage: server [OPTION]
A dynamic hash based accumulator designed for the Bitcoin UTXO set
The birdgenode server generates proofs and serves to the CSN node.

OPTIONS:
  -net=mainnet                 configure whether to use mainnet. Optional.
  -net=regtest                 configure whether to use regtest. Optional.

  -datadir="path/to/directory" set a custom DATADIR.
                               Defaults to the Bitcoin Core DATADIR path.
`

// bit of a hack. Standard flag lib doesn't allow flag.Parse(os.Args[2]).
// You need a subcommand to do so.
var optionCmd = flag.NewFlagSet("", flag.ExitOnError)
var netCmd = optionCmd.String("net", "testnet",
	"Target network. (testnet, regtest, mainnet) Usage: '-net=regtest'")
var dataDirCmd = optionCmd.String("datadir", "",
	`Set a custom datadir. Usage: "-datadir='path/to/directory'"`)

func main() {

	optionCmd.Parse(os.Args[1:])

	var param chaincfg.Params // wire.BitcoinNet
	if *netCmd == "testnet" {
		param = chaincfg.TestNet3Params
	} else if *netCmd == "regtest" {
		param = chaincfg.RegressionNetParams
	} else if *netCmd == "mainnet" {
		param = chaincfg.MainNetParams
	} else {
		fmt.Println("Invalid net flag given.")
		fmt.Println(msg)
		os.Exit(1)
	}

	// set datadir
	var dataDir string
	if *dataDirCmd == "" { // No custom datadir given by the user
		dataDir = util.GetBitcoinDataDir()
		if param.Name != chaincfg.MainNetParams.Name {
			dataDir = filepath.Join(dataDir, param.Name)
		}
		dataDir = filepath.Join(dataDir, "/blocks")
	} else {
		dataDir = *dataDirCmd // set dataDir to the one set by the user
	}

	//listen for SIGINT, SIGTERM, or SIGQUIT from the os
	sig := make(chan bool, 1)
	handleIntSig(sig)

	fmt.Printf("datadir is %s\n", dataDir)
	err := bridge.BuildProofs(param, dataDir, sig)
	if err != nil {
		fmt.Printf("Buildproofs error: %s\n", err.Error())
		panic("server halting")
	}

}

func handleIntSig(sig chan bool) {
	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)
	go func() {
		<-s
		sig <- true
	}()
}
