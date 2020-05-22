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
	"github.com/mit-dci/utreexo/csn"
	"github.com/mit-dci/utreexo/util"
)

var msg = `
Usage: cmd COMMAND [OPTION]
A dynamic hash based accumulator designed for the Bitcoin UTXO set

COMMANDS:
  genproofs                    generates proofs and serves to the CSN node.
                               this is the bridgenode.
  ibdsim                       simulates an ibd (initial block download).
                               this is the CSN node
OPTIONS:
  -net=testnet                 configure whether to use testnet. Optional.
  -net=regtest                 configure whether to use regtest. Optional.

  -datadir="path/to/directory" set a custom DATADIR.
                               Defaults to the Bitcoin Core DATADIR path.
`

// bit of a hack. Standard flag lib doesn't allow flag.Parse(os.Args[2]). You need a subcommand to do so.
var optionCmd = flag.NewFlagSet("", flag.ExitOnError)
var netCmd = optionCmd.String("net", "mainnet",
	"Target testnet or regtest instead of mainnet. Usage: '-net=regtest' or '-net=testnet'")
var dataDirCmd = optionCmd.String("datadir", "",
	`Set a custom datadir. Usage: "-datadir='path/to/directory'"`)

func main() {
	// check if enough arguments were given
	if len(os.Args) < 2 {
		fmt.Println(msg)
		os.Exit(1)
	}
<<<<<<< HEAD

=======
>>>>>>> simplify arguments to RunIBD, BuildProofs
	optionCmd.Parse(os.Args[2:])
<<<<<<< HEAD
	// set datadir
	var dataDir string
	if *dataDirCmd == "" { // No custom datadir given by the user
		dataDir = util.GetBitcoinDataDir()
	} else {
		dataDir = *dataDirCmd // set dataDir to the one set by the user
	}

	var ttldb, offsetfile string
	var net wire.BitcoinNet
	if *netCmd == "testnet" {
		ttldb = "testnet-ttldb"
		offsetfile = "testnet-offsetfile"
		dataDir = filepath.Join(dataDir, "testnet3")
		net = wire.TestNet3
	} else if *netCmd == "regtest" {
		ttldb = "regtest-ttldb"
		offsetfile = "regtest-offsetfile"
		dataDir = filepath.Join(dataDir, "regtest")
		net = wire.TestNet // yes, this is the name of regtest in lit
=======
	var param chaincfg.Params // wire.BitcoinNet
	if *netCmd == "testnet" {
		param = chaincfg.TestNet3Params
	} else if *netCmd == "regtest" {
		param = chaincfg.RegressionNetParams
>>>>>>> use coin params instead of just wire.net
	} else if *netCmd == "mainnet" {
		param = chaincfg.MainNetParams
	} else {
		fmt.Println("Invalid net flag given.")
		fmt.Println(msg)
		os.Exit(1)
	}
	//listen for SIGINT, SIGTERM, or SIGQUIT from the os
	sig := make(chan bool, 1)
	handleIntSig(sig)

	switch os.Args[1] {
	case "ibdsim":
		err := csn.RunIBD(&param, sig)
		if err != nil {
			panic(err)
		}
	case "genproofs":
<<<<<<< HEAD
<<<<<<< HEAD
		err := bridge.BuildProofs(dataDir, net, ttldb, offsetfile, sig)
=======
		err := bridge.BuildProofs(param, ttldb, offsetfile, sig)
>>>>>>> use coin params instead of just wire.net
=======
		err := bridge.BuildProofs(param, sig)
>>>>>>> simplify arguments to RunIBD, BuildProofs
		if err != nil {
			panic(err)
		}
	default:
		fmt.Println(msg)
		os.Exit(0)
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
