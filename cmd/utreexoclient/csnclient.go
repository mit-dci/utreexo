package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime/pprof"
	"runtime/trace"
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

  -host                        server to connect to.  Default to localhost
                               if you need a public server, try 35.188.186.244
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
var traceCmd = optionCmd.String("trace", "",
	`Enable trace. Usage: 'trace='path/to/file'`)
var watchAddr = optionCmd.String("watchaddr", "",
	`Address to watch & report transactions. Only bech32 p2wpkh supported`)
var remoteHost = optionCmd.String("host", "127.0.0.1",
	`remote server to connect to`)

var checkSig = optionCmd.Bool("checksig", true,
	`check signatures (slower)`)
var lookahead = optionCmd.Int("lookahead", 1000,
	`size of the look-ahead cache in blocks`)
var quitafter = optionCmd.Int("quitafter", -1,
	`quit ibd after n blocks. (for testing)`)

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

	if *traceCmd != "" {
		f, err := os.Create(*traceCmd)
		if err != nil {
			fmt.Println(err)
			fmt.Println(msg)
			os.Exit(1)
		}
		trace.Start(f)
	}

	// listen for SIGINT, SIGTERM, or SIGQUIT from the os
	sig := make(chan bool, 1)
	handleIntSig(sig, *cpuProfCmd, *traceCmd)

	err := csn.RunIBD(&param, *remoteHost, *watchAddr, *checkSig, *lookahead, sig, *quitafter)
	if err != nil {
		panic(err)
	}
}

func handleIntSig(sig chan bool, cpuProfCmd, traceCmd string) {
	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)
	go func() {
		<-s
		if cpuProfCmd != "" {
			pprof.StopCPUProfile()
		}
		if traceCmd != "" {
			trace.Stop()
		}
		sig <- true
	}()
}
