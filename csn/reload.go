package csn

import (
	"encoding/binary"
	"fmt"
	"os"

	"github.com/btcsuite/btcd/wire"
	"github.com/mit-dci/utreexo/accumulator"
	"github.com/mit-dci/utreexo/util"
)

// restorePollard restores the pollard from disk to memory.
// If starting anew, it just returns a empty pollard.
func restorePollard() (height int32, p accumulator.Pollard,
	utxos map[wire.OutPoint]util.LeafData, err error) {
	// Restore Pollard
	pollardFile, err := os.OpenFile(
		util.PollardFilePath, os.O_RDWR, 0600)
	if err != nil {
		return
	}

	// restore utxos
	var numUtxos uint32
	err = binary.Read(pollardFile, binary.BigEndian, &numUtxos)
	if err != nil {
		return
	}

	utxos = make(map[wire.OutPoint]util.LeafData)
	for ; numUtxos > 0; numUtxos-- {
		var utxo util.LeafData

		err = utxo.Deserialize(pollardFile)
		if err != nil {
			return
		}

		utxos[utxo.Outpoint] = utxo
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
func saveIBDsimData(csn *Csn) error {
	polFile, err := os.OpenFile(
		util.PollardFilePath, os.O_WRONLY, 0600)
	if err != nil {
		return err
	}

	// save all found utxos
	err = binary.Write(polFile, binary.BigEndian, uint32(len(csn.utxoStore)))
	if err != nil {
		return err
	}

	for _, utxo := range csn.utxoStore {
		err = utxo.Serialize(polFile)
		if err != nil {
			return err
		}
	}

	// write to the heightfile
	err = binary.Write(polFile, binary.BigEndian, csn.CurrentHeight)
	if err != nil {
		return err
	}
	err = csn.pollard.WritePollard(polFile)
	if err != nil {
		return err
	}
	return polFile.Close()
}
