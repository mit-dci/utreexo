package bridgenode

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcutil"
)

var helpMsg = `
Usage: server [OPTION]
A dynamic hash based accumulator designed for the Bitcoin UTXO set
The bridgenode server generates proofs and serves to the CSN node.

OPTIONS:
  -net=mainnet                 configure whether to use mainnet. Optional.
  -net=regtest                 configure whether to use regtest. Optional.
  -forest                      select forest type to use (ram, cow, cache, disk). Defaults to disk
  -datadir="path/to/directory" set a custom DATADIR.
                               Defaults to the Bitcoin Core DATADIR path
  -datadir="path/to/directory" set a custom DATADIR.
                               Defaults to the $HOME/.utreexo

  -cpuprof                     configure whether to use use cpu profiling
  -memprof                     configure whether to use use heap profiling
  -serve		       immediately serve whatever data is built
`

// bit of a hack. Standard flag lib doesn't allow flag.Parse(os.Args[2]).
// You need a subcommand to do so.
var (
	argCmd = flag.NewFlagSet("", flag.ExitOnError)
	netCmd = argCmd.String("net", "testnet",
		"Target network. (testnet, regtest, mainnet) Usage: '-net=regtest'")
	dataDirCmd = argCmd.String("datadir", "",
		`Set a custom datadir. Usage: "-datadir='path/to/directory'"`)
	bridgeDirCmd = argCmd.String("bridgedir", "",
		`Set a custom bridgenode datadir. Usage: "-bridgedir='path/to/directory"`)
	forestTypeCmd = argCmd.String("forest", "disk",
		`Set a forest type to use (cow, ram, disk, cache). Usage: "-forest=cow"`)
	cowMaxCache = argCmd.Int("cowmaxcache", 500,
		`how many treetables to cache with copy-on-write forest`)
	quitAtCmd = argCmd.Int("quitat", -1,
		`quit generating proofs after the given block height. (meant for testing)`)
	serve = argCmd.Bool("serve", false,
		`immediately start server without building or checking proof data`)
	noServeCmd = argCmd.Bool("noserve", false,
		`don't serve proofs after finishing generating them`)
	traceCmd = argCmd.String("trace", "",
		`Enable trace. Usage: 'trace='path/to/file'`)
	cpuProfCmd = argCmd.String("cpuprof", "",
		`Enable pprof cpu profiling. Usage: 'cpuprof='path/to/file'`)
	memProfCmd = argCmd.String("memprof", "",
		`Enable pprof heap profiling. Usage: 'memprof='path/to/file'`)
)

// utreexo home directory
var defaultHomeDir = btcutil.AppDataDir("utreexo", false)

type forestDir struct {
	base                            string
	forestFile                      string
	miscForestFile                  string
	forestLastSyncedBlockHeightFile string
	cowForestCurFile                string
	cowForestDir                    string
}

type proofDir struct {
	base        string
	pFile       string
	pOffsetFile string
	lastPOffset string
}

type offsetDir struct {
	base                      string
	offsetFile                string
	lastIndexOffsetHeightFile string
}

// All your utreexo bridgenode file paths in a nice and convinent struct
type utreeDir struct {
	offsetDir offsetDir
	proofDir  proofDir
	forestDir forestDir
	ttldb     string
}

// init an utreeDir with a selected basepath. Has all the names for the forest
func initUtreeDir(basePath string) utreeDir {
	offBase := filepath.Join(basePath, "offsetdata")
	off := offsetDir{
		base:                      offBase,
		offsetFile:                filepath.Join(offBase, "offsetfile.dat"),
		lastIndexOffsetHeightFile: filepath.Join(offBase, "lastindexoffsetheightfile.dat"),
	}

	proofBase := filepath.Join(basePath, "proofdata")
	proof := proofDir{
		base:        proofBase,
		pFile:       filepath.Join(proofBase, "proof.dat"),
		pOffsetFile: filepath.Join(proofBase, "proofoffset.dat"),
		lastPOffset: filepath.Join(proofBase, "lastproofoffset.dat"),
	}

	forestBase := filepath.Join(basePath, "forestdata")
	cowDir := filepath.Join(forestBase, "cow")
	forest := forestDir{
		base:                            forestBase,
		forestFile:                      filepath.Join(forestBase, "forestfile.dat"),
		miscForestFile:                  filepath.Join(forestBase, "miscforestfile.dat"),
		forestLastSyncedBlockHeightFile: filepath.Join(forestBase, "forestlastsyncedheight.dat"),
		cowForestDir:                    cowDir,
		cowForestCurFile:                filepath.Join(cowDir, "CURRENT"),
	}

	ttldb := filepath.Join(basePath, "ttldb")

	return utreeDir{
		offsetDir: off,
		proofDir:  proof,
		forestDir: forest,
		ttldb:     ttldb,
	}
}

// MakePaths makes the necessary paths for all files in a given network
func makePaths(dir utreeDir) {
	os.MkdirAll(dir.offsetDir.base, os.ModePerm)
	os.MkdirAll(dir.proofDir.base, os.ModePerm)
	os.MkdirAll(dir.forestDir.base, os.ModePerm)
	os.MkdirAll(dir.forestDir.cowForestDir, os.ModePerm)
}

type forestType int

const (
	// just keeps and accesses the forest as a big contiguous file
	diskForest forestType = iota

	// A cached version of the diskForest
	cacheForest

	// copy-on-write forest. Offers better performance than disk and cache. Maybe be unstable
	// in some edge cases.
	cowForest

	// keeps the entire forest in ram. doable if theres lots of ram (30GB+)
	ramForest
)

// all the configs for utreexoserver
type Config struct {
	// what params do we use? Different params depend on
	// which bitcoin network are we on (mainnet, testnet3, regnet)
	params chaincfg.Params

	// the block path from bitcoind's datadir we'll be directly reading from
	blockDir string

	// where will the bridgenode data be saved to?
	utreeDir utreeDir

	// type of the forest we're using
	forestType forestType

	// quitAfter syncing to this block height
	quitAt int

	// how much cache to allow for cowforest
	cowMaxCache int

	// just immidiately start serving what you have on disk
	serve bool

	// don't serve after generating proofs
	noServe bool

	// enable tracing
	TraceProf bool

	// enable cpu profiling
	CpuProf bool

	// enable memory profiling
	MemProf bool
}

// Parse parses the command line arguments and inits the server config
func Parse(args []string) (*Config, error) {
	argCmd.Parse(args)

	con := Config{}

	var dataDir string

	// set dataDir
	if *dataDirCmd == "" { // No custom datadir given by the user
		dataDir = btcutil.AppDataDir("bitcoin", true)
	} else {
		dataDir = *dataDirCmd // set dataDir to the one set by the user
	}

	var bridgeDir string

	// set bridgeDir
	if *bridgeDirCmd == "" { // No custom datadir given by the user
		bridgeDir = defaultHomeDir
	} else {
		bridgeDir = *bridgeDirCmd // set dataDir to the one set by the user
	}

	// set network
	if *netCmd == "testnet" {
		con.params = chaincfg.TestNet3Params
		con.blockDir = filepath.Join(
			filepath.Join(dataDir, chaincfg.TestNet3Params.Name),
			"blocks")
		base := filepath.Join(bridgeDir, chaincfg.TestNet3Params.Name)
		con.utreeDir = initUtreeDir(base)
	} else if *netCmd == "regtest" {
		con.params = chaincfg.RegressionNetParams
		con.blockDir = filepath.Join(
			filepath.Join(dataDir, chaincfg.RegressionNetParams.Name),
			"blocks")
		base := filepath.Join(bridgeDir, chaincfg.RegressionNetParams.Name)
		con.utreeDir = initUtreeDir(base)
	} else if *netCmd == "mainnet" {
		con.params = chaincfg.MainNetParams
		con.blockDir = filepath.Join(dataDir, "blocks")
		con.utreeDir = initUtreeDir(bridgeDir)
	} else {
		fmt.Println("Invalid/not supported net flag given.")
		fmt.Println(helpMsg)
		os.Exit(1)
	}

	makePaths(con.utreeDir)

	// set profiling
	if *cpuProfCmd != "" {
		con.CpuProf = true
	}
	if *memProfCmd != "" {
		con.MemProf = true
	}
	if *traceCmd != "" {
		con.TraceProf = true
	}

	switch *forestTypeCmd {
	case "disk":
		con.forestType = diskForest
	case "cache":
		con.forestType = cacheForest
	case "cow":
		con.forestType = cowForest
		con.cowMaxCache = *cowMaxCache
	case "ram":
		con.forestType = ramForest
	default:
		return nil, errWrongForestType(*forestTypeCmd)
	}

	con.quitAt = *quitAtCmd
	con.noServe = *noServeCmd
	con.serve = *serve

	return &con, nil
}
