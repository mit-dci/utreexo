package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/mit-dci/utreexo/csn"
)

var msg = `
Usage: client [OPTION]
A dynamic hash based accumulator designed for the Bitcoin UTXO set.
client performs ibd (initial block download) on the Bitcoin blockchain.
You can give a bech32 address to watch during IBD.

OPTIONS:
  -net=mainnet                 configure whether to use mainnet. Optional.
  -net=regtest                 configure whether to use regtest. Optional.

  -cpuprof                     configure whether to use use cpu profiling
  -memprof                     configure whether to use use heap profiling
`

// bit of a hack. Standard flag lib doesn't allow flag.Parse(os.Args[2]).
// You need a subcommand to do so.
var optionCmd = flag.NewFlagSet("", flag.ExitOnError)
var netCmd = optionCmd.String("net", "testnet",
	"Target network. (testnet, regtest, mainnet) Usage: '-net=regtest'")
var cpuProfCmd = optionCmd.String("cpuprof", "",
	`Enable pprof cpu profiling. Usage: 'cpuprof='path/to/file'`)
var memProfCmd = optionCmd.String("memprof", "",
	`Enable pprof heap profiling. Usage: 'memprof='path/to/file'`)
var watchAddr = optionCmd.String("watchaddr", "",
	`Address to watch & report transactions. Only bech32 p2wpkh supported`)
var checkSig = optionCmd.Bool("checksig", true,
	`check signatures (slower)`)

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

	if *cpuProfCmd != "" {
		f, err := os.Create(*cpuProfCmd)
		if err != nil {
			fmt.Println(err)
			fmt.Println(msg)
			os.Exit(1)
		}
		pprof.StartCPUProfile(f)
	}

	if *memProfCmd != "" {
		f, err := os.Create(*memProfCmd)
		if err != nil {
			fmt.Println(err)
			fmt.Println(msg)
			os.Exit(1)
		}
		pprof.WriteHeapProfile(f)
	}

	// listen for SIGINT, SIGTERM, or SIGQUIT from the os
	sig := make(chan bool, 1)
	handleIntSig(sig, *cpuProfCmd)

	err := csn.RunIBD(&param, *watchAddr, *checkSig, sig)
	if err != nil {
		panic(err)
	}
}

func handleIntSig(sig chan bool, cpuProfCmd string) {
	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)
	go func() {
		<-s
		if cpuProfCmd != "" {
			pprof.StopCPUProfile()
		}
		sig <- true
	}()
}
