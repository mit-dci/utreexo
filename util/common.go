package util

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"sync"
)

var (
	// littleEndian is a convenience variable since binary.LittleEndian is
	// quite long.
	littleEndian = binary.LittleEndian

	// bigEndian is a convenience variable since binary.BigEndian is quite
	// long.
	bigEndian = binary.BigEndian
)

// BinarySerializer is just a wrapper around a slice of bytes
type BinarySerializer struct {
	buf []byte
}

// BinarySerializerFree provides a free list of buffers to use for serializing and
// deserializing primitive integer values to and from io.Readers and io.Writers.
var BinarySerializerFree = sync.Pool{
	New: func() interface{} { return new(BinarySerializer) },
}

// NewSerializer allocates a new BinarySerializer struct or grabs a cached one
// from BinarySerializerFree
func NewSerializer() *BinarySerializer {
	b := BinarySerializerFree.Get().(*BinarySerializer)

	if b.buf == nil {
		b.buf = make([]byte, 8)
	}

	return b
}

// Free saves used BinarySerializer structs in ppFree; avoids an allocation per invocation.
func (bs *BinarySerializer) Free() {
	bs.buf = bs.buf[:0]
	BinarySerializerFree.Put(bs)
}

// Uint8 reads a single byte from the provided reader using a buffer from the
// free list and returns it as a uint8.
func (l *BinarySerializer) Uint8(r io.Reader) (uint8, error) {
	buf := l.buf[:1]
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, err
	}
	return buf[0], nil
}

// Uint16 reads two bytes from the provided reader using a buffer from the
// free list, converts it to a number using the provided byte order, and returns
// the resulting uint16.
func (l *BinarySerializer) Uint16(r io.Reader, byteOrder binary.ByteOrder) (uint16, error) {
	buf := l.buf[:2]
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, err
	}
	return byteOrder.Uint16(buf), nil
}

// Uint32 reads four bytes from the provided reader using a buffer from the
// free list, converts it to a number using the provided byte order, and returns
// the resulting uint32.
func (l *BinarySerializer) Uint32(r io.Reader, byteOrder binary.ByteOrder) (uint32, error) {
	buf := l.buf[:4]
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, err
	}
	return byteOrder.Uint32(buf), nil
}

// Uint64 reads eight bytes from the provided reader using a buffer from the
// free list, converts it to a number using the provided byte order, and returns
// the resulting uint64.
func (l *BinarySerializer) Uint64(r io.Reader, byteOrder binary.ByteOrder) (uint64, error) {
	buf := l.buf[:8]
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, err
	}
	return byteOrder.Uint64(buf), nil
}

// PutUint8 copies the provided uint8 into a buffer from the free list and
// writes the resulting byte to the given writer.
func (l *BinarySerializer) PutUint8(w io.Writer, val uint8) error {
	buf := l.buf[:1]
	buf[0] = val
	_, err := w.Write(buf)
	return err
}

// PutUint16 serializes the provided uint16 using the given byte order into a
// buffer from the free list and writes the resulting two bytes to the given
// writer.
func (l *BinarySerializer) PutUint16(w io.Writer, byteOrder binary.ByteOrder, val uint16) error {
	buf := l.buf[:2]
	byteOrder.PutUint16(buf, val)
	_, err := w.Write(buf)
	return err
}

// PutUint32 serializes the provided uint32 using the given byte order into a
// buffer from the free list and writes the resulting four bytes to the given
// writer.
func (l *BinarySerializer) PutUint32(w io.Writer, byteOrder binary.ByteOrder, val uint32) error {
	buf := l.buf[:4]
	byteOrder.PutUint32(buf, val)
	_, err := w.Write(buf)
	return err
}

// PutUint64 serializes the provided uint64 using the given byte order into a
// buffer from the free list and writes the resulting eight bytes to the given
// writer.
func (l *BinarySerializer) PutUint64(w io.Writer, byteOrder binary.ByteOrder, val uint64) error {
	buf := l.buf[:8]
	byteOrder.PutUint64(buf, val)
	_, err := w.Write(buf)
	return err
}

// errNonCanonicalVarInt is the common format string used for non-canonically
// encoded variable length integer errors.
var errNonCanonicalVarInt = "non-canonical varint %x - discriminant %x must " +
	"encode a value greater than %x"

// ReadVarInt reads a variable length integer from r and returns it as a uint64.
func ReadVarInt(r io.Reader, pver uint32) (uint64, error) {
	bs := NewSerializer()
	discriminant, err := bs.Uint8(r)
	bs.Free()
	if err != nil {
		return 0, err
	}

	var rv uint64
	switch discriminant {
	case 0xff:
		bs := NewSerializer()
		sv, err := bs.Uint64(r, littleEndian)
		bs.Free()
		if err != nil {
			return 0, err
		}
		rv = sv

		// The encoding is not canonical if the value could have been
		// encoded using fewer bytes.
		min := uint64(0x100000000)
		if rv < min {
			return 0, messageError("ReadVarInt", fmt.Sprintf(
				errNonCanonicalVarInt, rv, discriminant, min))
		}

	case 0xfe:
		bs := NewSerializer()
		sv, err := bs.Uint32(r, littleEndian)
		bs.Free()
		if err != nil {
			return 0, err
		}
		rv = uint64(sv)

		// The encoding is not canonical if the value could have been
		// encoded using fewer bytes.
		min := uint64(0x10000)
		if rv < min {
			return 0, messageError("ReadVarInt", fmt.Sprintf(
				errNonCanonicalVarInt, rv, discriminant, min))
		}

	case 0xfd:
		bs := NewSerializer()
		sv, err := bs.Uint16(r, littleEndian)
		bs.Free()
		if err != nil {
			return 0, err
		}
		rv = uint64(sv)

		// The encoding is not canonical if the value could have been
		// encoded using fewer bytes.
		min := uint64(0xfd)
		if rv < min {
			return 0, messageError("ReadVarInt", fmt.Sprintf(
				errNonCanonicalVarInt, rv, discriminant, min))
		}

	default:
		rv = uint64(discriminant)
	}

	return rv, nil
}

// WriteVarInt serializes val to w using a variable number of bytes depending
// on its value.
func WriteVarInt(w io.Writer, pver uint32, val uint64) error {
	if val < 0xfd {
		bs := NewSerializer()
		err := bs.PutUint8(w, uint8(val))
		bs.Free()
		return err
	}

	if val <= math.MaxUint16 {
		bs := NewSerializer()
		err := bs.PutUint8(w, 0xfd)
		if err != nil {
			bs.Free()
			return err
		}

		err = bs.PutUint16(w, littleEndian, uint16(val))
		bs.Free()
		return err
	}

	if val <= math.MaxUint32 {
		bs := NewSerializer()
		err := bs.PutUint8(w, 0xfe)
		if err != nil {
			bs.Free()
			return err
		}
		err = bs.PutUint32(w, littleEndian, uint32(val))
		bs.Free()
		return err
	}

	bs := NewSerializer()
	err := bs.PutUint8(w, 0xff)
	if err != nil {
		bs.Free()
		return err
	}
	err = bs.PutUint64(w, littleEndian, val)
	bs.Free()
	return err
}

// VarIntSerializeSize returns the number of bytes it would take to serialize
// val as a variable length integer.
func VarIntSerializeSize(val uint64) int {
	// The value is small enough to be represented by itself, so it's
	// just 1 byte.
	if val < 0xfd {
		return 1
	}

	// Discriminant 1 byte plus 2 bytes for the uint16.
	if val <= math.MaxUint16 {
		return 3
	}

	// Discriminant 1 byte plus 4 bytes for the uint32.
	if val <= math.MaxUint32 {
		return 5
	}

	// Discriminant 1 byte plus 8 bytes for the uint64.
	return 9
}
