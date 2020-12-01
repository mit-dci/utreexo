package csn

import (
	"flag"
	"strings"

	"github.com/btcsuite/btcd/chaincfg"
)

var PollardFilePath string = "pollardFile"

var HelpMsg = `
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
var (
	argCmd = flag.NewFlagSet("", flag.ExitOnError)
	netCmd = argCmd.String("net", "testnet",
		"Target network. (testnet, regtest, mainnet) Usage: '-net=regtest'")
	cpuProfCmd = argCmd.String("cpuprof", "",
		`Enable pprof cpu profiling. Usage: 'cpuprof='path/to/file'`)
	memProfCmd = argCmd.String("memprof", "",
		`Enable pprof heap profiling. Usage: 'memprof='path/to/file'`)
	traceCmd = argCmd.String("trace", "",
		`Enable trace. Usage: 'trace='path/to/file'`)
	watchAddr = argCmd.String("watchaddr", "",
		`Address to watch & report transactions. Only bech32 p2wpkh supported`)
	remoteHost = argCmd.String("host", "127.0.0.1",
		`remote server to connect to`)

	checkSig = argCmd.Bool("checksig", true,
		`check signatures (slower)`)
	lookahead = argCmd.Int("lookahead", 1000,
		`size of the look-ahead cache in blocks`)
	quitafter = argCmd.Int("quitafter", -1,
		`quit ibd after n blocks. (for testing)`)
)

type Config struct {
	params chaincfg.Params

	// host server
	remoteHost string

	// address to watch for txs
	watchAddr string

	// how much to remember
	lookAhead int

	// quitafter this many blocks
	quitafter int

	// Check Bitcoin tx signatures
	checkSig bool

	// enable tracing
	TraceProf string

	// enable cpu profiling
	CpuProf string

	// enable memory profiling
	MemProf string
}

func Parse(args []string) (*Config, error) {
	argCmd.Parse(args)

	cfg := Config{}

	if *netCmd == "testnet" {
		cfg.params = chaincfg.TestNet3Params
	} else if *netCmd == "regtest" {
		cfg.params = chaincfg.RegressionNetParams
	} else if *netCmd == "mainnet" {
		cfg.params = chaincfg.MainNetParams
	} else {
		return nil, errInvalidNetwork(*netCmd)
	}

	cfg.remoteHost = *remoteHost
	cfg.watchAddr = *watchAddr
	cfg.lookAhead = *lookahead
	cfg.quitafter = *quitafter
	cfg.checkSig = *checkSig

	// if no host was given, default to localhost
	if *remoteHost == "" {
		cfg.remoteHost = "127.0.0.1:8338"
	} else {
		if !strings.ContainsRune(*remoteHost, ':') {
			str := *remoteHost + ":8338"
			cfg.remoteHost = str
		}
	}

	cfg.CpuProf = *cpuProfCmd
	cfg.MemProf = *memProfCmd
	cfg.TraceProf = *traceCmd

	return &cfg, nil
}
