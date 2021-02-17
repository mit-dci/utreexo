package common

import (
	"encoding/binary"
	"io"
	"sync"
)

// FreeBytes is a wrapper around bytes
type FreeBytes struct {
	Bytes []byte
}

// FreeBytes the bytes to the FreeBytes pool
func (fb *FreeBytes) Free() {
	fb.Bytes = fb.Bytes[:0]
	FreeBytesPool.Put(fb)
}

// NewFreeBytes returns a parentHashBytes from the pool. Will allocate if the
// Pool returns parentHashBytes that doesn't have bytes allocated
func NewFreeBytes() *FreeBytes {
	fb := FreeBytesPool.Get().(*FreeBytes)

	if fb.Bytes == nil {
		// set minimum to 64 since that's what parentHash()
		// requires and parentHash() is called very frequently
		fb.Bytes = make([]byte, 0, 64)
	}

	return fb
}

// FreeBytesPool is the pool of bytes to recycle&relieve gc pressure.
var FreeBytesPool = sync.Pool{
	New: func() interface{} { return new(FreeBytes) },
}

// Uint8 reads a single byte from the provided reader using a buffer from the
// free list and returns it as a uint8.
func (fb *FreeBytes) Uint8(r io.Reader) (uint8, error) {
	buf := fb.Bytes[:1]
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, err
	}
	return buf[0], nil
}

// Uint16 reads two bytes from the provided reader using a buffer from the
// free list, converts it to a number using the provided byte order, and returns
// the resulting uint16.
func (fb *FreeBytes) Uint16(r io.Reader, byteOrder binary.ByteOrder) (uint16, error) {
	buf := fb.Bytes[:2]
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, err
	}
	return byteOrder.Uint16(buf), nil
}

// Uint32 reads four bytes from the provided reader using a buffer from the
// free list, converts it to a number using the provided byte order, and returns
// the resulting uint32.
func (fb *FreeBytes) Uint32(r io.Reader, byteOrder binary.ByteOrder) (uint32, error) {
	buf := fb.Bytes[:4]
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, err
	}
	return byteOrder.Uint32(buf), nil
}

// Uint64 reads eight bytes from the provided reader using a buffer from the
// free list, converts it to a number using the provided byte order, and returns
// the resulting uint64.
func (fb *FreeBytes) Uint64(r io.Reader, byteOrder binary.ByteOrder) (uint64, error) {
	buf := fb.Bytes[:8]
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, err
	}
	return byteOrder.Uint64(buf), nil
}

// PutUint8 copies the provided uint8 into a buffer from the free list and
// writes the resulting byte to the given writer.
func (fb *FreeBytes) PutUint8(w io.Writer, val uint8) error {
	buf := fb.Bytes[:1]
	buf[0] = val
	_, err := w.Write(buf)
	return err
}

// PutUint16 serializes the provided uint16 using the given byte order into a
// buffer from the free list and writes the resulting two bytes to the given
// writer.
func (fb *FreeBytes) PutUint16(w io.Writer, byteOrder binary.ByteOrder, val uint16) error {
	buf := fb.Bytes[:2]
	byteOrder.PutUint16(buf, val)
	_, err := w.Write(buf)
	return err
}

// PutUint32 serializes the provided uint32 using the given byte order into a
// buffer from the free list and writes the resulting four bytes to the given
// writer.
func (fb *FreeBytes) PutUint32(w io.Writer, byteOrder binary.ByteOrder, val uint32) error {
	buf := fb.Bytes[:4]
	byteOrder.PutUint32(buf, val)
	_, err := w.Write(buf)
	return err
}

// PutUint64 serializes the provided uint64 using the given byte order into a
// buffer from the free list and writes the resulting eight bytes to the given
// writer.
func (fb *FreeBytes) PutUint64(w io.Writer, byteOrder binary.ByteOrder, val uint64) error {
	buf := fb.Bytes[:8]
	byteOrder.PutUint64(buf, val)
	_, err := w.Write(buf)
	return err
}
