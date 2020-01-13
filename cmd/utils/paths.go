package simutil

import (
	"os"
	"path/filepath"
)

// Directory paths
var OffsetDirPath string = filepath.Join(".", "offsetdata")
var ProofDirPath string = filepath.Join(".", "proofdata")
var ForestDirPath string = filepath.Join(".", "forestdata")

// File paths

// offsetdata file paths
var OffsetFilePath string = filepath.Join(OffsetDirPath, "offsetfile")
var CurrentOffsetFilePath string = filepath.Join(OffsetDirPath, "currentoffsetfile")
var HeightFilePath string = filepath.Join(OffsetDirPath, "heightfile")

// proofdata file paths
var PFilePath string = filepath.Join(ProofDirPath, "proof.dat")
var POffsetFilePath string = filepath.Join(ProofDirPath, "proofoffset.dat")

// forestdata file paths
var ForestFilePath string = filepath.Join(ForestDirPath, "forestfile.dat")
var MiscForestFilePath string = filepath.Join(ForestDirPath, "miscforestfile.dat")

func MakePaths() {
	os.MkdirAll(OffsetDirPath, os.ModePerm)
	os.MkdirAll(ProofDirPath, os.ModePerm)
	os.MkdirAll(ForestDirPath, os.ModePerm)
}
