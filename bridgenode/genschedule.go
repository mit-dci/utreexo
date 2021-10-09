package bridgenode

import (
	"encoding/binary"
	"fmt"
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

	// example: read block 208 and report the ttls
	height := int32(208)

	// offset is the position in the ttlFile where block starts
	var offset int64
	// seek to the right place in the offset file
	_, err = ttlOffsetFile.Seek(int64(height)*8, 0)
	if err != nil {
		return err
	}

	// read the offset data
	err = binary.Read(ttlOffsetFile, binary.BigEndian, &offset)
	if err != nil {
		return err
	}

	// seek to the block start in the ttl file
	_, err = ttlFile.Seek(offset, 0)
	if err != nil {
		return err
	}

	// read number of ttls
	var ttlsInBlock int32
	err = binary.Read(ttlFile, binary.BigEndian, &ttlsInBlock)
	if err != nil {
		return err
	}

	ttls := make([]int32, ttlsInBlock)
	for i, _ := range ttls {
		binary.Read(ttlFile, binary.BigEndian, &ttls[i])
	}

	// print out those ttls
	for i, t := range ttls {
		fmt.Printf("height %d ttl %d = %d\n", height, i, t)
	}

	return nil
}
