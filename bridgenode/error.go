package bridgenode

import (
	"errors"
	"fmt"
)

var (
	ErrNoDataDir       = errors.New("No bitcoind datadir")
	ErrWrongForestType = errors.New("Invalid forest type of")
)

func errNoDataDir(path string) error {
	str := "in path: " + path
	return fmt.Errorf("%s %s", ErrNoDataDir, str)
}

func errWrongForestType(fType string) error {
	return fmt.Errorf("%s: %s", ErrWrongForestType, fType)
}
