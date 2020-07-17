package util

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
)

// Bitcoin Core DATADIR
var LinuxDataDir string = "/.bitcoin/"
var DarwinDataDir string = "/Library/Application Support/Bitcoin/"

// Directory paths
var OffsetDirPath string = filepath.Join(".", "utree/offsetdata")
var ProofDirPath string = filepath.Join(".", "utree/proofdata")
var ForestDirPath string = filepath.Join(".", "utree/forestdata")

// File paths
var PollardFilePath string = "pollardFile"

// offsetdata file paths
var OffsetFilePath string = filepath.Join(OffsetDirPath, "offsetfile")
var LastIndexOffsetHeightFilePath string = filepath.Join(OffsetDirPath,
	"lastindexoffsetheightfile")

// proofdata file paths
//
// Where the proofs for txs are stored
var PFilePath string = filepath.Join(ProofDirPath, "proof.dat")

// Where the index for a proof for a block is stored
var POffsetFilePath string = filepath.Join(ProofDirPath, "proofoffset.dat")

// For resuming purposes. Stores the last index that genproofs left at
var LastPOffsetFilePath string = filepath.Join(ProofDirPath, "lastproofoffset.dat")

// forestdata file paths
var ForestFilePath string = filepath.Join(ForestDirPath, "forestfile.dat")
var MiscForestFilePath string = filepath.Join(ForestDirPath, "miscforestfile.dat")
var ForestLastSyncedBlockHeightFilePath string = filepath.Join(ForestDirPath,
	"forestlastsyncedheight.dat")

// MakePaths makes the neccessary paths for all files
func MakePaths() {
	os.MkdirAll(OffsetDirPath, os.ModePerm)
	os.MkdirAll(ProofDirPath, os.ModePerm)
	os.MkdirAll(ForestDirPath, os.ModePerm)
}

// GetBitcoinDataDir grabs the user's Bitcoin DataDir. Doesn't support Windows or BSDs
func GetBitcoinDataDir() (dir string) {
	home := GetHomeDir()
	// runtime method for grabbing GO-OS (Go Operating System)
	switch runtime.GOOS {
	case "linux":
		dir = filepath.Join(home, LinuxDataDir)
	case "darwin":
		dir = filepath.Join(home, DarwinDataDir)
	default:
		str := fmt.Sprintf("%s unknown, must specify -datadir\n", runtime.GOOS)
		panic(str)
	}

	return
}

// GetHomeDir grabs the current user's home directory
func GetHomeDir() string {
	usr, err := user.Current()
	if err != nil {
		panic(err)
	}

	return usr.HomeDir
}
