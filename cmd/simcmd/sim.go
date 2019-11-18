package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/mit-dci/utreexo/cmd/blockparser"
	"github.com/mit-dci/utreexo/cmd/txottl"
	"github.com/mit-dci/utreexo/cmd/ibdsim"
)

var msg = `
Usage: simcmd COMMAND [OPTION]
A dynamic hash based accumulator designed for the Bitcoin UTXO set

Commands:
  parseblock     parses the blockdata in blocks/ and index/ directory. Outputs testnet.txos file and ttldb/
  txottlgen      appends txo lifetimes to testnet.txos. Outputs ttl.testnet.txos file
  ibdsim         simulates an initial block download with ttl.testnet.txos as an input
  genproofs      generates proofs from the ttl.testnet.txos file
  genhist        generates a histogram from the ttl.testnet.txos file
OPTIONS:
  ttlfn          can assign a ttlfn file name with 'ttlfn=filename'. Defaults to 'ttlfn=ttl.mainnet.txos'
  schedFileName  can assign a scheduled file to use with 'schedFileName=filename'. Defaults to 'schedFileName=schedule1pos.clr'
`

//commands
var parseblockCmd = flag.Bool("parseblock", false, "Parse the blockdata in blocks/ and index/ directory. Outputs testnet.txos file and ttldb/")
var txottlgenCmd = flag.Bool("txottlgen", false, "Appends txo lifetimes to testnet.txos file. Outputs ttl.testnet.txos file.")
var ibdsimCmd = flag.NewFlagSet("ibdsim", flag.ExitOnError)
var genproofsCmd = flag.NewFlagSet("genproofs", flag.ExitOnError)
var genhistCmd = flag.NewFlagSet("genhist", flag.ExitOnError)

//options

//bit of a hack. Stdandard flag lib doesn't allow flag.Parse(os.Args[2]). You need a subcommand to do so.
var optionCmd = flag.NewFlagSet("", flag.ExitOnError)
var ttlfn = optionCmd.String("ttlfn", "ttl.mainnet.txos", "assign a ttlfn file name with 'ttlfn=filename'")
var schedFileName = optionCmd.String("schedFileName", "schedule1pos.clr", "assign a scheduled file to use with 'schedFileName=filename'")

func main() {
	//check if enough arguments were given
	if len(os.Args) < 2 {
		fmt.Println(msg)
		os.Exit(1)
	}
	//listen for SIGINT, SIGTERM, or SIGQUIT from the os
	sig := make(chan bool, 1)
	handleIntSig(sig)

	switch os.Args[1] {
	case "parseblock":
		blockparser.Parser(sig)
	case "txottlgen":
		fmt.Println("Generating txo time to live...")
		txottl.ReadTTLdb(sig)
	case "ibdsim":
		optionCmd.Parse(os.Args[2:])
		err := ibdsim.RunIBD(*ttlfn, *schedFileName, sig)
		if err != nil {
			panic(err)
		}
	case "genproofs":
		optionCmd.Parse(os.Args[2:])
		fmt.Println("Building Proofs...")
		err := ibdsim.BuildProofs(*ttlfn, sig)
		if err != nil {
			panic(err)
		}
	case "genhist":
		optionCmd.Parse(os.Args[2:])
		err := ibdsim.Histogram(*ttlfn, sig)
		if err != nil {
			panic(err)
		}
	default:
		fmt.Println(msg)
		os.Exit(1)
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

