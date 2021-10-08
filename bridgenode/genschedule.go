package bridgenode

import (
	"os"
)

func BuildClairvoyantSchedule(cfg *Config, sig chan bool) error {

	// Open the offset file and ttl file which have already been saved
	ttlOffsetFile, err := os.Open(cfg.UtreeDir.TtlDir.OffsetFile)
	if err != nil {
		return err
	}
	ttlFile, err := os.Open(cfg.UtreeDir.TtlDir.ttlsetFile)
	if err != nil {
		return err
	}

	// the format of the offset file is a series of 8-byte big-endian integers
	// which say the byte that every block starts at.  To get the starting point
	// of the 100th block, go to the 800th byte of the offset file, and read
	// an int64.  Then seek to that location within the TTL file for the
	// series of 4-byte TTL values of block 100.

	ttlOffsetFile.Read(nil)
	ttlFile.Read(nil)

	return nil
}
