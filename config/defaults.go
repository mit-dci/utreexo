package config

import (
	"os"
	"os/user"
	"path/filepath"
	"runtime"
)

type Config struct {
	ChainDir string
	TxoFilename string
	LevelDBPath string
	MainnetTxo string
}

// Default config export
// TODO ovveride with config
var DefaultConfig = &Config {
	ChainDir: defaultChainDbDir(),
	TxoFilename: txoFilename,
	LevelDBPath: levelDBPath,
	MainnetTxo: mainnetTxo,
}

// DefaultDataDir is the default data directory to use for the databases and other
// persistence requirements.
func defaultChainDbDir() string {
	// Try to place the data folder in the user's home dir
	home := homeDir()
	if home != "" {
		if runtime.GOOS == "darwin" {
			return filepath.Join(home, "Library", "Bitcoin")
		} else if runtime.GOOS == "windows" {
			return filepath.Join(home, "AppData", "Roaming", "Bitcoin")
		} else {
			return filepath.Join(home, ".bitcoin")
		}
	}
	// As we cannot guess a stable location, return empty and handle later
	return ""
}

func homeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	if usr, err := user.Current(); err == nil {
		return usr.HomeDir
	}
	return ""
}