package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/btcsuite/btcd/wire"
	bridge "github.com/mit-dci/utreexo/cmd/bridgenode"
	"github.com/mit-dci/utreexo/cmd/csn"
)

var msg = `
Usage: utreexo COMMAND [OPTION]
A dynamic hash based accumulator designed for the Bitcoin UTXO set

Commands:
  ibdsim         simulates an initial block download with ttl.testnet.txos as an input
  genproofs      generates proofs from the ttl.testnet.txos file
OPTIONS:
  -net=testnet   configure whether to use testnet. Optional.
  -net=regtest   configure whether to use regtest. Optional.
`

// bit of a hack. Standard flag lib doesn't allow flag.Parse(os.Args[2]). You need a subcommand to do so.
var optionCmd = flag.NewFlagSet("", flag.ExitOnError)
var netCmd = optionCmd.String("net", "mainnet",
	"Target testnet or regtest instead of mainnet. Usage: '-net=regtest' or '-net=testnet'")

func main() {
	// check if enough arguments were given
	if len(os.Args) < 2 {
		fmt.Println(msg)
		os.Exit(1)
	}
	var ttldb, offsetfile string
	optionCmd.Parse(os.Args[2:])
	var net wire.BitcoinNet
	if *netCmd == "testnet" {
		ttldb = "testnet-ttldb"
		offsetfile = "testnet-offsetfile"
		net = wire.TestNet3
	} else if *netCmd == "regtest" {
		ttldb = "regtest-ttldb"
		offsetfile = "regtest-offsetfile"
		net = wire.TestNet // yes, this is the name of regtest in lit
	} else if *netCmd == "mainnet" {
		ttldb = "ttldb"
		offsetfile = "offsetfile"
		net = wire.MainNet
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
		err := csn.RunIBD(net, offsetfile, ttldb, sig)
		if err != nil {
			panic(err)
		}
	case "genproofs":
		err := bridge.BuildProofs(net, ttldb, offsetfile, sig)
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
