package csn

import (
	"encoding/binary"
	"fmt"
	"os"

	"github.com/mit-dci/utreexo/accumulator"
	"github.com/mit-dci/utreexo/util"
)

// restorePollard restores the pollard from disk to memory.
// If starting anew, it just returns a empty pollard.
func restorePollard() (height int32, p accumulator.Pollard, err error) {
	// Restore Pollard
	pollardFile, err := os.OpenFile(
		util.PollardFilePath, os.O_RDWR, 0600)
	if err != nil {
		return
	}
	err = binary.Read(pollardFile, binary.BigEndian, &height)
	if err != nil {
		return
	}

	err = p.RestorePollard(pollardFile)
	if err != nil {
		fmt.Printf("restore error\n")
		return
	}

	return
}

// saveIBDsimData saves the state of ibdsim so that when the
// user restarts, they'll be able to resume.
// Saves height for ibdsim and pollard itself
func saveIBDsimData(height int32, p accumulator.Pollard) error {
	polFile, err := os.OpenFile(
		util.PollardFilePath, os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	// write to the heightfile
	err = binary.Write(polFile, binary.BigEndian, height)
	if err != nil {
		return err
	}
	err = p.WritePollard(polFile)
	if err != nil {
		return err
	}
	return polFile.Close()
}
