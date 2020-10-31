package bridgenode

import (
	"errors"
	"fmt"
)

var (
	ErrNoDataDir         = errors.New("No bitcoind datadir")
	ErrTooManyForestType = errors.New("Only one forest type can be chosen")
)

func errTooManyForestType(num int) error {
	return fmt.Errorf("%s but %d were given", ErrTooManyForestType, num)
}

func errNoDataDir(path string) error {
	str := "in path: " + path
	return fmt.Errorf("%s %s", ErrNoDataDir, str)
}
