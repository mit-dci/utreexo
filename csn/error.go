package csn

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidNetwork = errors.New("Invalid/not supported net flag given")
)

func errInvalidNetwork(nType string) error {
	return fmt.Errorf("%s: %s", ErrInvalidNetwork, nType)
}
