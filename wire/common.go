// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/mit-dci/utreexo/util"
)

const (
	// MaxVarIntPayload is the maximum payload size for a variable length integer.
	MaxVarIntPayload = 9
)

// uint32Time represents a unix timestamp encoded with a uint32.  It is used as
// a way to signal the readElement function how to decode a timestamp into a Go
// time.Time since it is otherwise ambiguous.
type uint32Time time.Time

// int64Time represents a unix timestamp encoded with an int64.  It is used as
// a way to signal the readElement function how to decode a timestamp into a Go
// time.Time since it is otherwise ambiguous.
type int64Time time.Time

var (
	// littleEndian is a convenience variable since binary.LittleEndian is
	// quite long.
	littleEndian = binary.LittleEndian

	// bigEndian is a convenience variable since binary.BigEndian is quite
	// long.
	bigEndian = binary.BigEndian
)

// readElement reads the next sequence of bytes from r using little endian
// depending on the concrete type of element pointed to.
func readElement(r io.Reader, element interface{}) error {
	// Attempt to read the element based on the concrete type via fast
	// type assertions first.
	switch e := element.(type) {
	case *int32:
		bs := util.NewSerializer()
		rv, err := bs.Uint32(r, littleEndian)
		bs.Free()
		if err != nil {
			return err
		}
		*e = int32(rv)
		return nil

	case *uint32:
		bs := util.NewSerializer()
		rv, err := bs.Uint32(r, littleEndian)
		bs.Free()
		if err != nil {
			return err
		}
		*e = rv
		return nil

	case *int64:
		bs := util.NewSerializer()
		rv, err := bs.Uint64(r, littleEndian)
		bs.Free()
		if err != nil {
			return err
		}
		*e = int64(rv)
		return nil

	case *uint64:
		bs := util.NewSerializer()
		rv, err := bs.Uint64(r, littleEndian)
		bs.Free()
		if err != nil {
			return err
		}
		*e = rv
		return nil

	case *bool:
		bs := util.NewSerializer()
		rv, err := bs.Uint8(r)
		bs.Free()
		if err != nil {
			return err
		}
		if rv == 0x00 {
			*e = false
		} else {
			*e = true
		}
		return nil

	// Unix timestamp encoded as a uint32.
	case *uint32Time:
		bs := util.NewSerializer()
		rv, err := bs.Uint32(r, binary.LittleEndian)
		bs.Free()
		if err != nil {
			return err
		}
		*e = uint32Time(time.Unix(int64(rv), 0))
		return nil

	// Unix timestamp encoded as an int64.
	case *int64Time:
		bs := util.NewSerializer()
		rv, err := bs.Uint64(r, binary.LittleEndian)
		bs.Free()
		if err != nil {
			return err
		}
		*e = int64Time(time.Unix(int64(rv), 0))
		return nil

	// Message header checksum.
	case *[4]byte:
		_, err := io.ReadFull(r, e[:])
		if err != nil {
			return err
		}
		return nil

	// Message header command.
	case *[CommandSize]uint8:
		_, err := io.ReadFull(r, e[:])
		if err != nil {
			return err
		}
		return nil

	// IP address.
	case *[16]byte:
		_, err := io.ReadFull(r, e[:])
		if err != nil {
			return err
		}
		return nil

	case *chainhash.Hash:
		_, err := io.ReadFull(r, e[:])
		if err != nil {
			return err
		}
		return nil

	case *ServiceFlag:
		bs := util.NewSerializer()
		rv, err := bs.Uint64(r, littleEndian)
		bs.Free()
		if err != nil {
			return err
		}
		*e = ServiceFlag(rv)
		return nil

	case *InvType:
		bs := util.NewSerializer()
		rv, err := bs.Uint32(r, littleEndian)
		bs.Free()
		if err != nil {
			return err
		}
		*e = InvType(rv)
		return nil

	case *BitcoinNet:
		bs := util.NewSerializer()
		rv, err := bs.Uint32(r, littleEndian)
		bs.Free()
		if err != nil {
			return err
		}
		*e = BitcoinNet(rv)
		return nil

	case *RejectCode:
		bs := util.NewSerializer()
		rv, err := bs.Uint8(r)
		bs.Free()
		if err != nil {
			return err
		}
		*e = RejectCode(rv)
		return nil
	}

	// Fall back to the slower binary.Read if a fast path was not available
	// above.
	return binary.Read(r, littleEndian, element)
}

// readElements reads multiple items from r.  It is equivalent to multiple
// calls to readElement.
func readElements(r io.Reader, elements ...interface{}) error {
	for _, element := range elements {
		err := readElement(r, element)
		if err != nil {
			return err
		}
	}
	return nil
}

// writeElement writes the little endian representation of element to w.
func writeElement(w io.Writer, element interface{}) error {
	// Attempt to write the element based on the concrete type via fast
	// type assertions first.
	switch e := element.(type) {
	case int32:
		bs := util.NewSerializer()
		err := bs.PutUint32(w, littleEndian, uint32(e))
		bs.Free()
		return err

	case uint32:
		bs := util.NewSerializer()
		err := bs.PutUint32(w, littleEndian, e)
		bs.Free()
		return err

	case int64:
		bs := util.NewSerializer()
		err := bs.PutUint64(w, littleEndian, uint64(e))
		bs.Free()
		return err

	case uint64:
		bs := util.NewSerializer()
		err := bs.PutUint64(w, littleEndian, e)
		bs.Free()
		return err

	case bool:
		var err error
		if e {
			bs := util.NewSerializer()
			err = bs.PutUint8(w, 0x01)
			bs.Free()
			if err != nil {
				return err
			}
		} else {
			bs := util.NewSerializer()
			err = bs.PutUint8(w, 0x00)
			bs.Free()
			if err != nil {
				return err
			}
		}
		if err != nil {
			return err
		}
		return nil

	// Message header checksum.
	case [4]byte:
		_, err := w.Write(e[:])
		if err != nil {
			return err
		}
		return nil

	// Message header command.
	case [CommandSize]uint8:
		_, err := w.Write(e[:])
		if err != nil {
			return err
		}
		return nil

	// IP address.
	case [16]byte:
		_, err := w.Write(e[:])
		if err != nil {
			return err
		}
		return nil

	case *chainhash.Hash:
		_, err := w.Write(e[:])
		if err != nil {
			return err
		}
		return nil

	case ServiceFlag:
		bs := util.NewSerializer()
		err := bs.PutUint64(w, littleEndian, uint64(e))
		bs.Free()
		return err

	case InvType:
		bs := util.NewSerializer()
		err := bs.PutUint32(w, littleEndian, uint32(e))
		bs.Free()
		return err

	case BitcoinNet:
		bs := util.NewSerializer()
		err := bs.PutUint32(w, littleEndian, uint32(e))
		bs.Free()
		return err

	case RejectCode:
		bs := util.NewSerializer()
		err := bs.PutUint8(w, uint8(e))
		bs.Free()
		return err
	}

	// Fall back to the slower binary.Write if a fast path was not available
	// above.
	return binary.Write(w, littleEndian, element)
}

// writeElements writes multiple items to w.  It is equivalent to multiple
// calls to writeElement.
func writeElements(w io.Writer, elements ...interface{}) error {
	for _, element := range elements {
		err := writeElement(w, element)
		if err != nil {
			return err
		}
	}
	return nil
}

// ReadVarString reads a variable length string from r and returns it as a Go
// string.  A variable length string is encoded as a variable length integer
// containing the length of the string followed by the bytes that represent the
// string itself.  An error is returned if the length is greater than the
// maximum block payload size since it helps protect against memory exhaustion
// attacks and forced panics through malformed messages.
func ReadVarString(r io.Reader, pver uint32) (string, error) {
	count, err := util.ReadVarInt(r, pver)
	if err != nil {
		return "", err
	}

	// Prevent variable length strings that are larger than the maximum
	// message size.  It would be possible to cause memory exhaustion and
	// panics without a sane upper bound on this count.
	if count > MaxMessagePayload {
		str := fmt.Sprintf("variable length string is too long "+
			"[count %d, max %d]", count, MaxMessagePayload)
		return "", messageError("ReadVarString", str)
	}

	buf := make([]byte, count)
	_, err = io.ReadFull(r, buf)
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

// WriteVarString serializes str to w as a variable length integer containing
// the length of the string followed by the bytes that represent the string
// itself.
func WriteVarString(w io.Writer, pver uint32, str string) error {
	err := util.WriteVarInt(w, pver, uint64(len(str)))
	if err != nil {
		return err
	}
	_, err = w.Write([]byte(str))
	return err
}

// ReadVarBytes reads a variable length byte array.  A byte array is encoded
// as a varInt containing the length of the array followed by the bytes
// themselves.  An error is returned if the length is greater than the
// passed maxAllowed parameter which helps protect against memory exhaustion
// attacks and forced panics through malformed messages.  The fieldName
// parameter is only used for the error message so it provides more context in
// the error.
func ReadVarBytes(r io.Reader, pver uint32, maxAllowed uint32,
	fieldName string) ([]byte, error) {

	count, err := util.ReadVarInt(r, pver)
	if err != nil {
		return nil, err
	}

	// Prevent byte array larger than the max message size.  It would
	// be possible to cause memory exhaustion and panics without a sane
	// upper bound on this count.
	if count > uint64(maxAllowed) {
		str := fmt.Sprintf("%s is larger than the max allowed size "+
			"[count %d, max %d]", fieldName, count, maxAllowed)
		return nil, messageError("ReadVarBytes", str)
	}

	b := make([]byte, count)
	_, err = io.ReadFull(r, b)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// WriteVarBytes serializes a variable length byte array to w as a varInt
// containing the number of bytes, followed by the bytes themselves.
func WriteVarBytes(w io.Writer, pver uint32, bytes []byte) error {
	slen := uint64(len(bytes))
	err := util.WriteVarInt(w, pver, slen)
	if err != nil {
		return err
	}

	_, err = w.Write(bytes)
	return err
}

// randomUint64 returns a cryptographically random uint64 value.  This
// unexported version takes a reader primarily to ensure the error paths
// can be properly tested by passing a fake reader in the tests.
func randomUint64(r io.Reader) (uint64, error) {
	bs := util.NewSerializer()
	rv, err := bs.Uint64(r, bigEndian)
	bs.Free()
	if err != nil {
		return 0, err
	}
	return rv, nil
}

// RandomUint64 returns a cryptographically random uint64 value.
func RandomUint64() (uint64, error) {
	return randomUint64(rand.Reader)
}
