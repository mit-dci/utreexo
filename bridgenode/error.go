package bridgenode

import (
	"errors"
	"fmt"
)

var (
	ErrNoDataDir       = errors.New("No bitcoind datadir")
	ErrWrongForestType = errors.New("Invalid forest type of")
	ErrInvalidNetwork  = errors.New("Invalid/not supported net flag given")
	ErrBuildProofs     = errors.New("BuildProofs error")
	ErrArchiveServer   = errors.New("ArchiveServer error")
	ErrKeyNotFound     = errors.New("Key not found")
)

func errNoDataDir(path string) error {
	str := "in path: " + path
	return fmt.Errorf("%s: %s", ErrNoDataDir, str)
}

func errWrongForestType(fType string) error {
	return fmt.Errorf("%s: %s", ErrWrongForestType, fType)
}

func errInvalidNetwork(nType string) error {
	return fmt.Errorf("%s: %s", ErrInvalidNetwork, nType)
}

func errBuildProofs(s error) error {
	return fmt.Errorf("%s: %s", ErrBuildProofs, s)
}

func errArchiveServer(s error) error {
	return fmt.Errorf("%s: %s", ErrArchiveServer, s)
}

func errKeyNotFound(s error) error {
	return fmt.Errorf("%s: %s", ErrKeyNotFound, s)
}
