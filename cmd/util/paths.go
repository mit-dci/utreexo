package util

import (
	"os"
	"path/filepath"
)

// Directory paths
var OffsetDirPath string = filepath.Join(".", "offsetdata")
var ProofDirPath string = filepath.Join(".", "proofdata")
var ForestDirPath string = filepath.Join(".", "forestdata")
var PollardDirPath string = filepath.Join(".", "pollarddata")

// File paths

// offsetdata file paths
var OffsetFilePath string = filepath.Join(OffsetDirPath, "offsetfile")
var LastIndexOffsetHeightFilePath string = filepath.Join(OffsetDirPath, "lastindexoffsetheightfile")

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
var ForestLastSyncedBlockHeightFilePath string = filepath.Join(ForestDirPath, "forestlastsyncedheight.dat")

// pollard data file paths
var PollardFilePath string = filepath.Join(PollardDirPath, "pollardfile.dat")
var PollardHeightFilePath string = filepath.Join(PollardDirPath, "pollardheight.dat")

// MakePaths makes the neccessary paths for all files
func MakePaths() {
	os.MkdirAll(OffsetDirPath, os.ModePerm)
	os.MkdirAll(ProofDirPath, os.ModePerm)
	os.MkdirAll(ForestDirPath, os.ModePerm)
	os.MkdirAll(PollardDirPath, os.ModePerm)
}
