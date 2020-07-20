package bridgenode

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/btcsuite/btcd/wire"
	"github.com/mit-dci/utreexo/util"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	dbutil "github.com/syndtr/goleveldb/leveldb/util"
)

// Wire Protocol version
// Some btcd lib requires this as an argument
// Technically the version is 70013 but many btcd
// code is passing 0 on some Deserialization methods
const pver uint32 = 0

// MaxMessagePayload is the maximum bytes a message can be regardless of other
// individual limits imposed by messages themselves.
const MaxMessagePayload = (1024 * 1024 * 32) // 32MB

// RawHeaderData is used for blk*.dat offsetfile building
// Used for ordering blocks as they aren't stored in order in the blk files.
// Includes 32 bytes of sha256 hash along with other variables
// needed for offsetfile building.
type RawHeaderData struct {
	// CurrentHeaderHash is the double hashed 32 byte header
	CurrentHeaderHash [32]byte
	// Prevhash is the 32 byte previous header included in the 80byte header.
	// Needed for ordering
	Prevhash [32]byte
	// FileNum is the blk*.dat file number
	FileNum [4]byte
	// Offset is where it is in the .dat file.
	Offset [4]byte
	// revblock position
	UndoPos uint32
}

// BlockReader is a wrapper around GetRawBlockFromFile so that the process
// can be made into a goroutine. As long as it's running, it keeps sending
// the entire blocktxs and height to bchan with TxToWrite type.
// It also puts in the proofs.  This will run on the archive server, and the
// data will be sent over the network to the CSN.
func BlockAndRevReader(
	blockChan chan BlockAndRev, dataDir, cOffsetFile string,
	maxHeight, curHeight int32) {

	var offsetFilePath string

	// If empty string is given, just use the default path
	// If not, then use the custom one given
	if cOffsetFile == "" {
		offsetFilePath = util.OffsetFilePath
	} else {
		offsetFilePath = cOffsetFile
	}

	for curHeight < maxHeight {
		blocks, revs, err := GetRawBlocksFromDisk(curHeight, 100000, offsetFilePath, dataDir)
		if err != nil {
			fmt.Printf(err.Error())
			// close(blockChan)
			return
		}

		for i := 0; i < len(blocks); i++ {
			bnr := BlockAndRev{
				Height: curHeight,
				Blk:    blocks[i],
				Rev:    revs[i],
			}
			blockChan <- bnr
			curHeight++
		}
	}
}

// GetRawBlocksFromDisk retrives multiple consecutive blocks starting at height `startAt`.
// `count` is a upper limit for the number of blocks read.
// Only blocks that are contained in the same blk file are returned.
func GetRawBlocksFromDisk(startAt int32, count int32, offsetFileName string,
	blockDir string) (blocks []wire.MsgBlock, revs []RevBlock, err error) {
	if startAt == 0 {
		err = fmt.Errorf("Block 0 is not in blk files or utxo set")
		return
	}
	startAt--

	if count <= 0 {
		return
	}

	offsetFile, err := os.Open(offsetFileName)
	if err != nil {
		return
	}
	defer offsetFile.Close() // file always closes

	// offset file consists of 12 bytes per block
	// tipnum * 12 gives us the correct position for that block
	_, err = offsetFile.Seek(int64(12*startAt), 0)
	if err != nil {
		return
	}
	offsetReader := bufio.NewReaderSize(offsetFile, int(12*count))

	offsets := make([]uint32, count)
	revOffsets := make([]uint32, count)

	var datFileNum uint32
	// offsetsRead holds the number of blocks < count that are loacated in
	// blk file with number `datFileNum`
	offsetsRead := uint32(0)
	for ; offsetsRead < uint32(count); offsetsRead++ {
		var datFileNumTmp uint32
		err = binary.Read(offsetReader, binary.BigEndian, &datFileNumTmp)
		if err != nil {
			break
		}
		if offsetsRead > 0 && datFileNumTmp != datFileNum {
			// break if the block is located in a different blk file
			break
		}
		datFileNum = datFileNumTmp
		err = binary.Read(offsetReader, binary.BigEndian, &offsets[offsetsRead])
		if err != nil {
			return
		}
		err = binary.Read(offsetReader, binary.BigEndian, &revOffsets[offsetsRead])
		if err != nil {
			return
		}
	}

	if offsetsRead == 0 {
		return
	}

	blockFile, err := os.Open(filepath.Join(blockDir,
		fmt.Sprintf("blk%05d.dat", datFileNum)))
	if err != nil {
		return
	}
	defer blockFile.Close()

	// Read all block data needed for the blocks into memory.
	// 1<<27 = 128MB
	blockData := make([]byte, 1<<27)
	_, err = blockFile.Read(blockData)
	if err != nil {
		return
	}

	revFile, err := os.Open(filepath.Join(blockDir,
		fmt.Sprintf("rev%05d.dat", datFileNum)))
	if err != nil {
		return
	}
	defer revFile.Close()

	// Read all rev data needed for the blocks into memory.
	// 1<<27 = 128MB
	revData := make([]byte, 1<<27)
	_, err = revFile.Read(revData)
	if err != nil {
		return
	}

	blocks = make([]wire.MsgBlock, offsetsRead)
	revs = make([]RevBlock, offsetsRead)
	skip := make([]byte, 8)
	for i := uint32(0); i < offsetsRead; i++ {
		blockBuf := bytes.NewBuffer(blockData[offsets[i]:])
		// skip 8 bytes, magic bytes + load size.
		blockBuf.Read(skip)
		// TODO this is probably expensive. fix
		err = blocks[i].Deserialize(blockBuf)
		if err != nil {
			return
		}

		revBuf := bytes.NewBuffer(revData[revOffsets[i]:])
		err = revs[i].Deserialize(revBuf)
		if err != nil {
			return
		}
	}

	return
}

// GetRawBlocksFromFile reads the blocks from the given .dat file and
// returns those blocks.
// Skips the genesis block. If you search for block 0, it will give you
// block 1.
func GetRawBlockFromFile(tipnum int32, offsetFileName string, blockDir string) (
	block wire.MsgBlock, rBlock RevBlock, err error) {
	if tipnum == 0 {
		err = fmt.Errorf("Block 0 is not in blk files or utxo set")
		return
	}
	tipnum--

	var datFile, offset, revOffset uint32

	offsetFile, err := os.Open(offsetFileName)
	if err != nil {
		return
	}
	defer offsetFile.Close() // file always closes

	// offset file consists of 12 bytes per block
	// tipnum * 12 gives us the correct position for that block
	_, err = offsetFile.Seek(int64(12*tipnum), 0)
	if err != nil {
		return
	}

	// Read file and offset for the block
	err = binary.Read(offsetFile, binary.BigEndian, &datFile)
	if err != nil {
		return
	}
	err = binary.Read(offsetFile, binary.BigEndian, &offset)
	if err != nil {
		return
	}
	err = binary.Read(offsetFile, binary.BigEndian, &revOffset)
	if err != nil {
		return
	}

	blockFName := fmt.Sprintf("blk%05d.dat", datFile)
	revFName := fmt.Sprintf("rev%05d.dat", datFile)

	bDir := filepath.Join(blockDir, blockFName)
	rDir := filepath.Join(blockDir, revFName)

	blockFile, err := os.Open(bDir)
	if err != nil {
		return
	}
	defer blockFile.Close() // file always closes

	// +8 skips the 8 bytes of magicbytes and load size
	_, err = blockFile.Seek(int64(offset)+8, 0)
	if err != nil {
		return
	}

	// TODO this is probably expensive. fix
	err = block.Deserialize(blockFile)
	if err != nil {
		return
	}

	revFile, err := os.Open(rDir)
	if err != nil {
		return
	}
	defer revFile.Close() // file always closes

	revFile.Seek(int64(revOffset), 0)
	err = rBlock.Deserialize(revFile)
	if err != nil {
		return
	}

	return
}

// BlockAndRev is a regular block and a rev block stuck together
type BlockAndRev struct {
	Height int32
	Rev    RevBlock
	Blk    wire.MsgBlock
}

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
	Magic [4]byte   // Network magic bytes
	Size  [4]byte   // size of the BlockUndo record
	Txs   []*TxUndo // acutal undo record
	Hash  [32]byte  // 32 byte double sha256 hash of the block
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
	Amount int64

	// Whether if the TxInUndo is a coinbase or not
	// Not actually included in the rev*.dat files
	Coinbase bool
}

// Deserialize takes a reader and reads a single block
// Only initializes the Block var in RevBlock
func (rb *RevBlock) Deserialize(r io.Reader) error {
	txCount, err := wire.ReadVarInt(r, pver)
	if err != nil {
		return err
	}

	for i := uint64(0); i < txCount; i++ {
		var tx TxUndo
		err := tx.Deserialize(r)
		if err != nil {
			return err
		}
		rb.Txs = append(rb.Txs, &tx)
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
	// if ti.Height > 0 {
	_, err := wire.ReadVarInt(r, pver)
	if err != nil {
		return err
	}

	// if varint != 0 {
	// return fmt.Errorf("varint is %d", varint)
	// }
	// ti.Varint = varint
	// }

	amount, _ := deserializeVLQ(r)
	ti.Amount = decompressTxOutAmount(amount)

	ti.PKScript = decompressScript(r)
	if ti.PKScript == nil {
		return fmt.Errorf("nil pkscript on h %d, pks %x\n", ti.Height, ti.PKScript)

	}

	return nil
}

// OpenIndexFile returns the db with only read only option enabled
func OpenIndexFile(dataDir string) (*leveldb.DB, error) {
	var indexDir string
	indexDir = filepath.Join(dataDir, "/index")
	// Read-only and no compression on
	// Bitcoin Core uses uncompressed leveldb. If that db is
	// opened EVEN ONCE, with compression on, the user will
	// have to re-index (takes hours, maybe days)
	o := opt.Options{ReadOnly: true, Compression: opt.NoCompression}
	lvdb, err := leveldb.OpenFile(indexDir, &o)
	if err != nil {
		return nil, fmt.Errorf("can't open %s\n", indexDir)
	}

	return lvdb, nil
}

// CBlockFileIndex is a reimplementation of the Bitcoin Core
// class CBlockFileIndex
type CBlockFileIndex struct {
	Version int32  // nVersion info of the block
	Height  int32  // Height of the block
	Status  int32  // validation status of the block in Bitcoin Core
	TxCount int32  // tx count in the block
	File    int32  // file num
	DataPos uint32 // blk*.dat file offset
	UndoPos uint32 // rev*.dat file offset
}

// Block status bits
const (
	//! Unused.
	BlockValidUnknown int32 = 0
	// Reserved
	BlockValidReserved int32 = 1

	//! All parent headers found, difficulty matches, timestamp >= median previous, checkpoint. Implies all parents
	//! are also at least TREE.
	BlockValidTree int32 = 2

	/**
	 * Only first tx is coinbase, 2 <= coinbase input script length <= 100, transactions valid, no duplicate txids,
	 * sigops, size, merkle root. Implies all parents are at least TREE but not necessarily TRANSACTIONS. When all
	 * parent blocks also have TRANSACTIONS, CBlockIndex::nChainTx will be set.
	 */
	BlockValidTransactions int32 = 3

	//! Outputs do not overspend inputs, no double spends, coinbase output ok, no immature coinbase spends, BIP30.
	//! Implies all parents are also at least CHAIN.
	BlockValidChain int32 = 4

	//! Scripts & signatures ok. Implies all parents are also at least SCRIPTS.
	BlockValidScripts int32 = 5

	//! All validity bits.
	BlockValidMask int32 = BlockValidReserved | BlockValidTree | BlockValidTransactions |
		BlockValidChain | BlockValidScripts

	BlockHaveData int32 = 8  //!< full block available in blk*.dat
	BlockHaveUndo int32 = 16 //!< undo data available in rev*.dat
	BlockHaveMask int32 = BlockHaveData | BlockHaveUndo

	BlockFailedValid int32 = 32 //!< stage after last reached validness failed
	BlockFailedChild int32 = 64 //!< descends from failed block
	BlockFailedMask  int32 = BlockFailedValid | BlockFailedChild

	BlockOptWitness int32 = 128 //!< block data in blk*.data was received with a witness-enforcing client
)

// BufferDB buffers the leveldb key values into map in memory
func BufferDB(lvdb *leveldb.DB) map[[32]byte]uint32 {
	bufDB := make(map[[32]byte]uint32)
	var header [32]byte

	iter := lvdb.NewIterator(dbutil.BytesPrefix([]byte{0x62}), nil)
	for iter.Next() {
		copy(header[:], iter.Key()[1:])
		cbIdx := ReadCBlockFileIndex(bytes.NewReader(iter.Value()))

		if cbIdx.Status&BlockHaveUndo > 0 {
			bufDB[header] = cbIdx.UndoPos
		}
	}

	iter.Release()
	err := iter.Error()
	if err != nil {
		panic(err)
	}

	return bufDB
}

func ReadCBlockFileIndex(r io.ReadSeeker) (cbIdx CBlockFileIndex) {
	// not sure if nVersion is correct...?
	nVersion, _ := deserializeVLQ(r)
	cbIdx.Version = int32(nVersion)

	nHeight, _ := deserializeVLQ(r)
	cbIdx.Height = int32(nHeight)

	// nStatus is incorrect but everything else correct. Probably reading this wrong
	nStatus, _ := deserializeVLQ(r)
	cbIdx.Status = int32(nStatus)

	nTx, _ := deserializeVLQ(r)
	cbIdx.TxCount = int32(nTx)

	nFile, _ := deserializeVLQ(r)
	cbIdx.File = int32(nFile)

	nDataPos, _ := deserializeVLQ(r)
	cbIdx.DataPos = uint32(nDataPos)

	nUndoPos, _ := deserializeVLQ(r)
	cbIdx.UndoPos = uint32(nUndoPos)

	// Need to seek 3 bytes if you're fetching the actual
	// header information. Not sure why it's needed but there's
	// no documentation to be found on the Bitcoin Core side
	// r.Seek(3, 1)

	return cbIdx
}

func min(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}
