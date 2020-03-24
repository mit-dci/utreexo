package util

import (
	"fmt"
	"io"
	"os"

	"github.com/btcsuite/btcd/wire"
)

// Wire Protocol version
// Some btcd lib requires this as an argument
// Technically the version is 70013 but many btcd
// code is passing 0 on some Deserialization methods
const pver uint32 = 0

// MaxMessagePayload is the maximum bytes a message can be regardless of other
// individual limits imposed by messages themselves.
const MaxMessagePayload = (1024 * 1024 * 32) // 32MB

/*
 * All types here follow the Bitcoin Core implementation of the
 * Undo blocks. One difference is that all the vectors are replaced
 * with slices. This is just a language difference.
 *
 * Compression/Decompression and VarInt functions are all taken/using
 * btcsuite packages.
 */

// RevBlock is the structure of how a block is stored in the
// rev*.dat file the Bitcoin Core generates
type RevBlock struct {
	Magic [4]byte    // Network magic bytes
	Size  [4]byte    // size of the BlockUndo record
	Block *BlockUndo // acutal undo record
	Hash  [32]byte   // 32 byte double sha256 hash of the block
}

// BlockUndo is the slice of undo information about transactions
// Excludes the coinbase transaction
// see github.com/bitcoin/bitcoin/src/undo.h
type BlockUndo struct {
	Tx []*TxUndo
}

// TxUndo contains the TxInUndo records.
// see github.com/bitcoin/bitcoin/src/undo.h
type TxUndo struct {
	TxIn []*TxInUndo
}

// TxInUndo is the stucture of the undo transaction
// Eveything is uncompressed here
// see github.com/bitcoin/bitcoin/src/undo.h
type TxInUndo struct {
	Height int32

	// Version of the original tx that created this tx
	Varint uint64

	// scriptPubKey of the spent UTXO
	PKScript []byte

	// Value of the spent UTXO
	Amount uint64

	// Whether if the TxInUndo is a coinbase or not
	// Not actually included in the rev*.dat files
	Coinbase bool
}

// GetRevBlock gets a single block from the rev*.dat file given the height
func GetRevBlock(height int32, revOffsetFileName string) (
	rBlock RevBlock, err error) {

	var datFile [4]byte
	var offset [4]byte

	offsetFile, err := os.Open(revOffsetFileName)
	if err != nil {
		return rBlock, err
	}

	// offset file consists of 8 bytes per block
	// height * 8 gives us the correct position for that block
	offsetFile.Seek(int64(8*height), 0)

	// Read file and offset for the block
	offsetFile.Read(datFile[:])
	offsetFile.Read(offset[:])

	fileName := fmt.Sprintf("rev%05d.dat", int(BtU32(datFile[:])))

	f, err := os.Open(fileName)
	if err != nil {
		return rBlock, err
	}
	// +8 skips the 8 bytes of magicbytes and load size
	f.Seek(int64(BtU32(offset[:])+8), 0)

	err = rBlock.Deserialize(f)
	if err != nil {
		return rBlock, err
	}
	f.Close()
	offsetFile.Close()

	return
}

// Deserialize takes a reader and reads a single block
// Only initializes the Block var in RevBlock
func (rb *RevBlock) Deserialize(r io.Reader) error {
	txCount, err := wire.ReadVarInt(r, pver)
	if err != nil {
		return err
	}

	rb.Block = new(BlockUndo)
	for i := uint64(0); i < txCount; i++ {
		var tx TxUndo
		err := tx.Deserialize(r)
		if err != nil {
			return err
		}
		rb.Block.Tx = append(rb.Block.Tx, &tx)
	}
	return nil
}

// Deserialize takes a reader and reads all the TxUndo data
func (tx *TxUndo) Deserialize(r io.Reader) error {

	// Read the Variable Integer
	count, err := wire.ReadVarInt(r, pver)
	if err != nil {
		return err
	}

	for i := uint64(0); i < count; i++ {
		var in TxInUndo
		err := readTxInUndo(r, &in)
		if err != nil {
			return err
		}
		// NOTE: Sanity check. Should never be true
		if in.PKScript == nil {
			fmt.Println("WARNING nil script")
		}
		tx.TxIn = append(tx.TxIn, &in)
	}
	return nil
}

// readTxInUndo reads all the TxInUndo from the reader to the passed in txInUndo
// variable
func readTxInUndo(r io.Reader, ti *TxInUndo) error {
	// nCode is how height is saved to the rev files
	nCode, _ := deserializeVLQ(r)
	ti.Height = int32(nCode / 2) // Height is saved as actual height * 2
	ti.Coinbase = nCode&1 == 1   // Coinbase is odd. Saved as height * 2 + 1

	// Only TxInUndos that have the height greater than 0
	// Has varint that isn't 0. see
	// github.com/bitcoin/bitcoin/blob/9cc7eba1b5651195c05473004c00021fe3856f30/src/undo.h#L42
	if ti.Height > 0 {
		varint, err := wire.ReadVarInt(r, pver)
		if err != nil {
			return err
		}
		if varint != 0 {
			return fmt.Errorf("varint is %d\n", varint)
		}
		ti.Varint = varint
	}

	amount, _ := deserializeVLQ(r)
	ti.Amount = decompressTxOutAmount(amount)

	pkscript := decompressScript(r)
	ti.PKScript = pkscript

	return nil
}

// BuildRevOffsetFile builds an offset file for rev*.dat files
// Just an index.
func BuildRevOffsetFile() error {
	offsetFile, err := os.OpenFile(RevOffsetFilePath,
		os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer offsetFile.Close()

	for fileNum := uint32(0); ; fileNum++ {
		fileName := fmt.Sprintf("rev%05d.dat", fileNum)
		_, err := os.Stat(fileName)
		if os.IsNotExist(err) {
			fmt.Printf("%s doesn't exist; done building\n",
				fileName)
			break
		}

		err = writeOffset(fileNum, offsetFile)
		if err != nil {
			return err
		}
	}
	return nil
}

// writeOffset reads the magic bytes from the rev*.dat files to make an index of
// each individual revblock
func writeOffset(fileNum uint32, offsetFile *os.File) error {
	fileName := fmt.Sprintf("rev%05d.dat", fileNum)
	f, err := os.Open(fileName)
	if err != nil {
		return err
	}
	defer f.Close()

	fStat, err := f.Stat()
	if err != nil {
		return err
	}
	fSize := fStat.Size()

	offset := uint32(0)
	loc := int64(0)
	// Read until the location of the offset is bigger than that of the file size
	for loc < fSize {
		// check if Bitcoin magic bytes were read
		var magicbytes [4]byte
		_, err := f.Read(magicbytes[:])
		if err != nil {
			panic(err)
		}
		if CheckMagicByte(magicbytes) == false {
			fmt.Println("non-magic byte read" +
				"May have an incomplete rev*.dat file")
			break
		}

		// read the 4 byte size of the load of the block
		var size [4]byte
		_, err = f.Read(size[:])
		if err != nil {
			return err
		}

		// Write the .dat file name and the
		// offset the block can be found at
		offsetFile.Write(U32tB(fileNum))
		offsetFile.Write(U32tB(offset))

		// offset for the next block from the current position
		// skip the 32 bytes of double sha hash of the rev block
		i, err := f.Seek(int64(LBtU32(size[:]))+32, 1)
		if err != nil {
			return err
		}
		// set offset
		offset = uint32(i)
		// advance location
		loc += i

	}
	return nil
}
