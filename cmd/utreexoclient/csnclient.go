package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/mit-dci/utreexo/csn"
)

var msg = `
Usage: client COMMAND [OPTION]
A dynamic hash based accumulator designed for the Bitcoin UTXO set

COMMANDS:
The client performs ibd (initial block download).

OPTIONS:
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
	// check if enough arguments were given
	// if len(os.Args) < 1 {
	// fmt.Println(msg)
	// os.Exit(1)
	// }

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
	optionCmd.Parse(os.Args[1:])
	// set datadir

	//listen for SIGINT, SIGTERM, or SIGQUIT from the os
	sig := make(chan bool, 1)
	handleIntSig(sig)

	err := csn.RunIBD(&param, sig)
	if err != nil {
		panic(err)
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
