package accumulator

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// leafSize is a [32]byte hash (sha256).
// Length is always 32.
const leafSize = 32

// This is for error checking. Rather expensive if you have it on
// Worth it to be on for now
var sanity bool = true

// ForestData is the thing that holds all the hashes in the forest.  Could
// be in a file, or in ram, or maybe something else.
type ForestData interface {
	// returns the hash value at the given position
	read(pos uint64) Hash

	// writes the given hash at the given position
	write(pos uint64, h Hash)

	// for the given two positions, swap the hash values
	swapHash(a, b uint64)

	// given positions a and b, take the width value (w) and swap
	// all the positions widthin it.
	swapHashRange(a, b, w uint64)

	// returns how many leaves the current forest can hold
	size() uint64

	// allocate more space to the forest. newSize should be in leaf count (bottom row of the forest)
	// can't resize down
	resize(newSize uint64) // make it have a new size (bigger)

	// closes the forest-on-disk for stopping
	close()
}

// ********************************************* forest in ram

type ramForestData struct {
	m []byte
}

// TODO it reads a lot of empty locations which can't be good

// reads from specified location.  If you read beyond the bounds that's on you
// and it'll crash
func (r *ramForestData) read(pos uint64) (h Hash) {
	// if r.m[pos] == empty {
	// 	fmt.Printf("\tuseless read empty at pos %d\n", pos)
	// }
	pos <<= 5
	copy(h[:], r.m[pos:pos+leafSize])
	return
}

// writeHash writes a hash.  Don't go out of bounds.
func (r *ramForestData) write(pos uint64, h Hash) {
	// if h == empty {
	// 	fmt.Printf("\tWARNING!! write empty at pos %d\n", pos)
	// }
	pos <<= 5
	copy(r.m[pos:pos+leafSize], h[:])
}

// TODO there's lots of empty writes as well, mostly in resize?  Anyway could
// be optimized away.

// swapHash swaps 2 hashes.  Don't go out of bounds.
func (r *ramForestData) swapHash(a, b uint64) {
	r.swapHashRange(a, b, 1) // just calls swap range..
}

// swapHashRange swaps 2 continuous ranges of hashes.  Don't go out of bounds.
// fast but uses more ram
func (r *ramForestData) swapHashRange(a, b, w uint64) {
	// fmt.Printf("swaprange %d %d %d\t", a, b, w)
	a <<= 5
	b <<= 5
	w <<= 5
	temp := make([]byte, w)
	copy(temp[:], r.m[a:a+w])

	copy(r.m[a:a+w], r.m[b:b+w])
	copy(r.m[b:b+w], temp[:])
}

// size gives you the size of the forest
func (r *ramForestData) size() uint64 {
	return uint64(len(r.m) / leafSize)
}

// resize makes the forest bigger (never gets smaller so don't try)
func (r *ramForestData) resize(newSize uint64) {
	r.m = append(r.m, make([]byte, (newSize-r.size())*leafSize)...)
}

func (r *ramForestData) close() {
	// nothing to do here fro a ram forest.
}

// ********************************************* forest on disk

// This is the same concept as forestRows, except for treeBlocks.
// Fixed as a treeBlockRow cannot increase. You just make another treeBlock if the
// current one isn't enough.
const treeBlockRows = 6

// rowPerTreeBlock is the rows a treeBlock holds
// 7 is chosen as a tree with height 6 will contain 7 rows (row 0 to row 6)
const rowPerTreeBlock = treeBlockRows + 1

// Number for the amount of treeBlocks to go into a table
const treeBlockPerTable = 1024

// Number of leaves that a treeBlock holds
const nodesPerTreeBlock = (2 << treeBlockRows) - 1

// Number of nodes that a treeTable holds
const nodesPerTreeTable = nodesPerTreeBlock * treeBlockPerTable

// extension for the forest files on disk. Stands for, "Utreexo Forest
// On Disk
var extension string = ".ufod"

var (
	ErrorCorruptManifest = errors.New("Manifest is corrupted. Recovery needed")
)

func errorCorruptManifest() error { return ErrorCorruptManifest }

// metadata holds the temporary data about the CowForest that isn't saved
// to disk
type metadata struct {
	// maxCachedTables is the maximum amount of tables to have on memory
	// when there is more, everything is flushed
	maxCachedTreeTables int

	// fBasePath is the base directory for the .ufod files
	fBasePath string

	// staleFiles are the files that are not part of the latest forest state
	// these should be cleaned up.
	staleFiles []uint64
}

// manifest is the structure saved on disk for loading the current
// utreexo forest
// FIXME fix padding
type manifest struct {
	// The current allocated rows in forest
	forestRows uint8

	// The current allocated treeBlockRows in the CowForest
	treeBlockRows uint8

	// The latest synced Bitcoin block height
	currentBlockHeight int32

	// The number following 'MANIFEST'
	// A new one will be written on every db open
	currentManifestNum uint64

	// The current .ufod file number. This is incremented on every
	// new treeTable
	fileNum uint64

	// The latest synced Bitcoin block hash
	currentBlockHash Hash

	// location holds the on-disk fileNum for the treeTables. 1st array
	// holds the treeBlockRow info and the seoncd holds the offset
	location [][]uint64
}

// commit creates a new manifest version and commits it and removes the old manifest
// The commit is atomic in that only when the commit was successful, the
// old manifest is removed.
func (m *manifest) commit(basePath string) error {
	manifestNum := m.currentManifestNum + 1
	fName := fmt.Sprintf("MANIFEST-%06d", manifestNum)
	fPath := filepath.Join(basePath, fName)

	// Create new manifest on disk
	fNewManifest, err := os.OpenFile(fPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return err
	}

	// This is the bytes to be written
	var buf []byte

	// 1. Append forestRows
	buf = append(buf, byte(m.forestRows))

	if verbose {
		fmt.Println("buf len1 ", len(buf))
		fmt.Println("forestRows:", m.forestRows)
	}

	// 2. Append currentBlockHeight
	var bHeight [4]byte
	binary.LittleEndian.PutUint32(bHeight[:], uint32(m.currentBlockHeight))
	buf = append(buf, bHeight[:]...)

	if verbose {
		fmt.Println("buf len2 ", len(buf))
		fmt.Println(buf)
		fmt.Println("currentBlockHeight:", m.currentBlockHeight)
	}

	// 3. Append fileNum
	var fNum [8]byte
	binary.LittleEndian.PutUint64(fNum[:], uint64(m.fileNum))
	buf = append(buf, fNum[:]...)

	if verbose {
		fmt.Println("buf len3 ", len(buf))
		fmt.Println("fileNum", m.fileNum)
	}

	// 4. Append currentBlockHash
	buf = append(buf, m.currentBlockHash[:]...)

	if verbose {
		fmt.Println(buf)
		fmt.Println("buf len4 ", len(buf))
		fmt.Println("curBH", m.currentBlockHash)
		fmt.Println(m.location)
	}

	// 5. Append locations
	for _, row := range m.location {
		// append the length of the row
		uint32Buf := make([]byte, 4)

		binary.LittleEndian.PutUint32(uint32Buf[:], uint32(len(row)))

		if verbose {
			fmt.Println("rowsize", len(row))
		}
		buf = append(buf, uint32Buf...)

		// append the actual row
		rowBytes := []byte{}

		for _, element := range row {
			uint64Buf := make([]byte, binary.MaxVarintLen64)
			binary.LittleEndian.PutUint64(uint64Buf, element)
			rowBytes = append(rowBytes, uint64Buf...)
		}

		buf = append(buf, rowBytes...)
	}

	if verbose {
		fmt.Println(len(buf))
	}

	_, err = fNewManifest.Write(buf)
	if err != nil {
		return err
	}

	// Overwrite the current manifest number in CURRENT
	curFileName := filepath.Join(basePath, "CURRENT")
	fCurrent, err := os.OpenFile(curFileName, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	fNameByteArray := []byte(fName)
	_, err = fCurrent.WriteAt(fNameByteArray, 0)
	if err != nil {
		return err
	}

	if m.currentManifestNum > 0 {
		// Remove old manifest
		fOldName := fmt.Sprintf("MANIFEST-%06d", m.currentManifestNum)
		OldFPath := filepath.Join(basePath, fOldName)
		err = os.Remove(OldFPath)
		if err != nil {
			e := fmt.Errorf("ErrOldManifestNotRemoved")
			return e
		}
	}

	return nil
}

// load loades the manifest from the disk
func (m *manifest) load(path string) error {
	curFileName := filepath.Join(path, "CURRENT")

	curFile, err := os.OpenFile(curFileName, os.O_RDONLY, 0666)
	if err != nil {
		return err
	}

	manifestBytes, err := ioutil.ReadAll(curFile)
	if err != nil {
		return err
	}

	maniFName := string(manifestBytes[:])

	maniFilePath := filepath.Join(path, maniFName)

	maniNumString := strings.Replace(
		maniFName, "MANIFEST-", "", -1)

	// set manifest num
	m.currentManifestNum, err = strconv.ParseUint(
		maniNumString, 10, 64)
	if err != nil {
		return err
	}

	maniFile, err := os.Open(maniFilePath)
	if err != nil {
		return err
	}

	// 45 bytes are all that's needed to load except for the locations
	buf := make([]byte, 45)

	_, err = maniFile.Read(buf)
	if err != nil {
		return err
	}

	// 1. Read forestRows
	m.forestRows = uint8(buf[0])

	if verbose {
		fmt.Println("forestRows:", m.forestRows)
	}

	// 2. Read currentBlockHeight
	m.currentBlockHeight = int32(binary.LittleEndian.Uint32(buf[1:5]))

	if verbose {
		fmt.Println("currentBlockHeight:", m.currentBlockHeight)
	}

	// 3. Read fileNum
	m.fileNum = binary.LittleEndian.Uint64(buf[5:13])
	if verbose {
		fmt.Println("fileNum", m.fileNum)
	}

	// 4. Read currentBlockHash
	copy(m.currentBlockHash[:], buf[13:45])

	if verbose {
		fmt.Println("curBlockH", m.currentBlockHash)
	}

	var treeBlockRow int
	// 5. Append locations
	for {
		sizeBuf := make([]byte, 4)

		_, err := maniFile.Read(sizeBuf)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		m.location = append(m.location, []uint64{})

		rowSize := binary.LittleEndian.Uint32(sizeBuf)

		if verbose {
			fmt.Println("rowsize", rowSize)
		}
		rowBytes := make([]byte, rowSize*binary.MaxVarintLen64)

		_, err = maniFile.Read(rowBytes)
		if err != nil {
			return err
		}

		for i := uint32(0); i < rowSize; i++ {
			start := i * binary.MaxVarintLen64
			rowToAppend := binary.LittleEndian.Uint64(
				rowBytes[start : start+binary.MaxVarintLen64])
			m.location[treeBlockRow] = append(m.location[treeBlockRow], rowToAppend)
		}
		treeBlockRow++
	}
	if verbose {
		fmt.Println(m.location)
	}

	return nil
}

// treeBlock is a representation of a forestRows 6 utreexo tree.
type treeBlock struct {
	leaves [nodesPerTreeBlock]Hash
}

// converts a treeBlock to byte slice
func (tb *treeBlock) serialize() []byte {
	var buf []byte

	for _, leaf := range tb.leaves {
		buf = append(buf, leaf[:]...)
	}
	return buf
}

// takes a byte slice and spits out a treeBlock
func deserializeTreeBlock(buf []byte) *treeBlock {
	tb := new(treeBlock)
	for i := 0; i < nodesPerTreeBlock; i++ {
		offset := i * leafSize
		copy(tb.leaves[i][:], buf[offset:offset+leafSize])
	}
	return tb
}

// getTreeBlockPos grabs the relevant treeBlock position.
func getTreeBlockPos(pos uint64, forestRows uint8) (
	treeBlockRow uint8, treeBlockOffset uint64, err error) {
	// maxPossiblePosition is the upper limit for the position that a given tree
	// can hold. Should error out if a requested position is greater than it.
	maxPossiblePosition := getRowOffset(forestRows, forestRows)
	if pos > maxPossiblePosition {
		err = fmt.Errorf("Position requested is more than the forest can hold\n"+
			"Requested: %d, MaxPossible: %d, forestRows: %d\n",
			pos, maxPossiblePosition, forestRows)
		return
	}

	// The row that the current position is on
	row := detectRow(pos, forestRows)

	// treeBlockRow is the "row" representation of a treeBlock forestRows
	// treeBlockRow of 0 indicates that it's the bottommost treeBlock.
	// Our current version has forestRows of 6 for the bottommost and 13
	// for the treeBlock above that. This is as there are 7 rows per treeBlock
	// ex: 0~6, 7~13, 14~20
	treeBlockRow = row / rowPerTreeBlock

	// get position relevant to the row, not the entire forest
	// ex:
	// 06
	// |-------\
	// 04      05
	// |---\   |---\
	// 00  01  02  03
	//
	// row 0 stays the same. Everything else changes
	// position 04 -> 00, 05 -> 01, 06 -> 00
	rowOffset := getRowOffset(row, forestRows)
	localRowPos := pos - rowOffset

	leafCount := 1 << forestRows       // total leaves in the forest
	nodeCountAtRow := leafCount >> row // total nodes in this row

	nodeCountAtTreeBlockRow := leafCount >> (treeBlockRow * rowPerTreeBlock) // total nodes in this TreeBlockrow

	// If there are less nodes at row than a treeBlock holds, it means that there
	// are empty leaves in that single treeBlock
	var treeBlockCountAtRow int
	if nodeCountAtTreeBlockRow < (1 << treeBlockRows) {
		// Only 1 treeBlock, the top treeBlock, may be sparse.
		treeBlockCountAtRow = 1
	} else {
		treeBlockCountAtRow = nodeCountAtTreeBlockRow / (1 << treeBlockRows)
	}

	// In a given row, how many leaves go into a treeBlock of that row?
	// For exmaple, a forest with:
	// row = 1, rowPerTreeBlock = 3
	// maxLeafPerTreeBlockAtRow = 2
	//
	// another would be
	// row = 0, rowPerTreeBlock = 3
	// maxLeafPerTreeBlockAtRow = 4
	maxLeafPerTreeBlockAtRow := nodeCountAtRow / treeBlockCountAtRow

	treeBlockOffset = localRowPos / uint64(maxLeafPerTreeBlockAtRow)

	return
}

// getRowOffset returns the first position of that row
// ex:
// 14
// |---------------\
// 12              13
// |-------\       |-------\
// 08      09      10      11
// |---\   |---\   |---\   |---\
// 00  01  02  03  04  05  06  07
//
// 8 = getRowOffset(1, 3)
// 12 = getRowOffset(2, 3)
func getRowOffset(row, forestRows uint8) uint64 {
	// 2 << forestRows is 2 more than the max poisition
	// to get the correct offset for a given row,
	// subtract (2 << `row complement of forestRows`) from (2 << forestRows)
	offset := (2 << forestRows) - (2 << (forestRows - row))
	return uint64(offset)
}

// Translate a global position to its local position. This is the leaf position
// inside a treeBlock. If a treeBlock is of forestRows 6, it's a range of
// 0-126
func gPosToLocPos(gPos, offset uint64, treeBlockRow, forestRows uint8) (
	uint8, uint64) {
	// Sanity check to see if called with something it can't hold
	if gPos > getRowOffset(forestRows, forestRows) {
		s := fmt.Errorf("pos of %d is greater than the max of what forestRows"+
			"%d can hold\n", gPos, forestRows)
		// TODO better to return err
		panic(s)
	}

	// which row is the node in in the entire forest?
	globalRow := detectRow(gPos, forestRows)

	// the first position in the globalRow
	rowOffset := getRowOffset(globalRow, forestRows)

	//
	rowPos := gPos - rowOffset

	// total leaves in the forest
	leafCount := 1 << forestRows

	// total nodes in this row
	nodeCountAtRow := leafCount >> globalRow

	// total nodes in this treeBlockRow
	nodeCountAtTreeBlockRow := leafCount >> (treeBlockRow * rowPerTreeBlock)

	// If there are less nodes at row than a treeBlock holds, it means that there
	// are empty leaves in that single treeBlock
	var treeBlockCountAtRow int
	if nodeCountAtTreeBlockRow < (1 << treeBlockRows) {
		// Only 1 treeBlock, the top treeBlock, may be sparse.
		treeBlockCountAtRow = 1
	} else {
		treeBlockCountAtRow = nodeCountAtTreeBlockRow / (1 << treeBlockRows)
	}

	rowBlockOffset := offset * uint64(nodeCountAtRow/treeBlockCountAtRow)
	locPos := rowPos - rowBlockOffset
	locRow := (globalRow - (rowPerTreeBlock * treeBlockRow))

	return locRow, locPos
}

// treeTable is a group of treeBlocks that are sorted on disk
// A treeTable only contains the treeBlocks that are of the same row
// The included treeBlocks are sorted then stored onto disk
type treeTable struct {
	//treeBlockRow uint8
	// memTreeBlocks is the treeBlocks that are stored in memory before they are
	// written to disk. This is helpful as older treeBlocks get less and
	// less likely to be accessed as stated in 5.7 of the utreexo paper
	// NOTE 1024 is the current value of stored treeBlocks per treeTable
	// this value may change/can be changed
	memTreeBlocks [treeBlockPerTable]*treeBlock
}

func (tt *treeTable) serialize() (uint16, []byte) {
	var buf []byte
	var treeBlockCount uint16
	for _, tb := range tt.memTreeBlocks {
		if tb == nil {
			break
		}

		buf = append(buf, tb.serialize()...)

		treeBlockCount++
	}

	return treeBlockCount, buf
}

// given a fileNum on disk, deserialize that table
func deserializeTreeTable(treeSlice io.Reader, treeBlockCount uint16) (*treeTable, error) {
	tt := new(treeTable)
	tbBytes := make([]byte, nodesPerTreeBlock*leafSize)
	var totallen int

	for i := uint16(0); i < treeBlockCount; i++ {
		_, err := treeSlice.Read(tbBytes)
		if err != nil {
			return nil, err
		}
		tt.memTreeBlocks[i] = deserializeTreeBlock(tbBytes)
		totallen += len(tbBytes)

	}

	return tt, nil
}

func newTreeTable() *treeTable {
	memBlocks := make([]*treeBlock, treeBlockPerTable)
	tt := new(treeTable)
	copy(tt.memTreeBlocks[:], memBlocks)
	return tt
}

// Shorthand for copy-on-write. Unfortuntely, it doesn't go moo
type cowForest struct {
	// cachedTreeTables are the in-memory tables that are not yet committed to disk
	// TODO flush these after a certain number is in memory
	cachedTreeTables map[uint64]*treeTable

	// all the data that isn't saved to disk
	meta metadata

	// manifest contains all the necessary metadata for fetching
	// utreexo nodes
	manifest manifest
}

// initalize returns a cowForest with a maxCachedTables value set
func initialize(path string, maxTreeTableCount int) (*cowForest, error) {
	m := metadata{
		fBasePath:           path,
		maxCachedTreeTables: maxTreeTableCount,
	}

	cow := cowForest{
		meta: m,
	}

	cow.cachedTreeTables = make(map[uint64]*treeTable)
	cow.manifest.location = append(cow.manifest.location, []uint64{})

	err := os.MkdirAll(path, os.ModePerm)
	if err != nil {
		panic(err)
	}
	return &cow, nil
}

// loads an existing cowForest
func loadCowForest(path string, maxTreeTableCount int) (*cowForest, error) {
	maniToLoad := new(manifest)

	err := maniToLoad.load(path)
	if err != nil {
		return nil, err
	}

	m := metadata{
		fBasePath:           path,
		maxCachedTreeTables: maxTreeTableCount,
	}

	cow := cowForest{
		manifest: *maniToLoad,
		meta:     m,
	}

	cow.cachedTreeTables = make(map[uint64]*treeTable)

	return &cow, nil
}

// Read takes a position and forestRows to return the Hash of that leaf
func (cow *cowForest) read(pos uint64) Hash {
	// Steps for Read go as such:
	//
	// 1. Fetch the relevant treeTable/treeBlock
	// 	a. Check if it's in memory. If not, go to disk
	// 2. Fetch the relevant treeBlock
	// 3. Fetch the leaf

	treeBlockRow, treeBlockOffset, err := getTreeBlockPos(pos, cow.manifest.forestRows)
	if err != nil {
		panic(err)
	}

	treeTableOffset := treeBlockOffset / treeBlockPerTable

	// grab the treeTable location. This is just a number for the .ufod file
	location := cow.manifest.location[treeBlockRow][treeTableOffset]

	// check if it exists in memory
	table, found := cow.cachedTreeTables[location]

	// Table is not in memory
	if !found {
		// Load the treeTable onto memory. This maps the table to the location
		err = cow.load(location)
		if err != nil {
			// TODO better to return err
			panic(err)
		}
		table = cow.cachedTreeTables[location]

		// advance fileNum and set as new file
		cow.manifest.fileNum++
		cow.manifest.location[treeBlockRow][treeTableOffset] =
			cow.manifest.fileNum

		// set as table
		cow.cachedTreeTables[cow.manifest.fileNum] = table

		// delete old key
		delete(cow.cachedTreeTables, location)

		// add file to be cleaned up
		cow.meta.staleFiles = append(
			cow.meta.staleFiles, location)
	}

	tb := table.memTreeBlocks[treeBlockOffset%treeBlockPerTable]
	if tb == nil {
		tb = new(treeBlock)
	}

	locRow, localPos := gPosToLocPos(
		pos, treeBlockOffset, treeBlockRow, cow.manifest.forestRows)
	fetch := localPos + getRowOffset(locRow, treeBlockRows)

	hash := tb.leaves[fetch]

	if verbose {
		fmt.Printf("READ RETURN on pos: %d with hash: %x\n",
			pos, hash)
	}
	return hash
}

// write changes the in-memory representation of the relevant treeBlock
// NOTE The treeBlocks on disk are not changed. commit must be called for that
func (cow *cowForest) write(pos uint64, h Hash) {
	if verbose {
		fmt.Printf("WRITE CALLED on pos: %d with hash: %x\n", pos, h)
	}

	if pos > getRowOffset(cow.manifest.forestRows, cow.manifest.forestRows) {
		s := fmt.Errorf("pos of %d is greater than the max of what forestRows"+
			"%d can hold\n", pos, cow.manifest.forestRows)
		panic(s)
	}

	treeBlockRow, treeBlockOffset, err := getTreeBlockPos(pos, cow.manifest.forestRows)
	if err != nil {
		// TODO better to return err
		panic(err)
	}
	treeTableOffset := treeBlockOffset / treeBlockPerTable

	// grab the treeTable location. This is just a number for the .ufod file
	location := cow.manifest.location[treeBlockRow][treeTableOffset]

	// check if it exists in memory
	table, found := cow.cachedTreeTables[location]

	// if not found in memory, load then update the fileNum
	if !found {
		err = cow.load(location)
		if err != nil {
			// TODO better to return err
			panic(err)
		}
		table = cow.cachedTreeTables[location]

		// advance fileNum and set as new file
		cow.manifest.fileNum++
		cow.manifest.location[treeBlockRow][treeTableOffset] =
			cow.manifest.fileNum

		// set as table
		cow.cachedTreeTables[cow.manifest.fileNum] = table

		// delete old key
		delete(cow.cachedTreeTables, location)

		// add file to be cleaned up
		cow.meta.staleFiles = append(
			cow.meta.staleFiles, location)
	}

	// there there is no treeBlock, then attach one
	if table.memTreeBlocks[treeBlockOffset%treeBlockPerTable] == nil {
		if verbose {
			fmt.Println("TREEBLOCK IS NIL")
		}
		table.memTreeBlocks[treeBlockOffset%treeBlockPerTable] = new(treeBlock)
	}

	locRow, localPos := gPosToLocPos(
		pos, treeBlockOffset, treeBlockRow, cow.manifest.forestRows)

	fetch := localPos + getRowOffset(locRow, treeBlockRows)
	table.memTreeBlocks[treeBlockOffset%treeBlockPerTable].leaves[fetch] = h

	// sanity checking
	if sanity {
		compH := cow.read(pos)
		if compH != h {
			fmt.Printf("%x\n", table.memTreeBlocks[treeBlockOffset%treeBlockPerTable].leaves[fetch])
			err := fmt.Errorf("the hash written doesn't equal what's supposed to be written"+
				"written %x but read %x\n", h, compH)
			panic(err)
		}
	}
	if verbose {
		fmt.Println("WRITE RETURN")
	}
}

// swapHash takes in two hashes and atomically swaps them.
// NOTE The treeBlocks on disk are not changed. commit must be called for that
func (cow *cowForest) swapHash(a, b uint64) {
	aHash := cow.read(a)
	bHash := cow.read(b)

	cow.write(a, bHash)
	cow.write(b, aHash)
}

func (cow *cowForest) swapHashRange(a, b, w uint64) {
	aHashes := make([]Hash, 0, w+1) // +1 as to include a
	bHashes := make([]Hash, 0, w+1) // +1 as to include b

	for i := a; i < a+w; i++ {
		aHashes = append(aHashes, cow.read(i))
	}

	for i := b; i < b+w; i++ {
		bHashes = append(bHashes, cow.read(i))
	}

	var counter int
	for i := a; i < a+w; i++ {
		cow.write(i, bHashes[counter])
		counter++
	}

	counter = 0
	for i := b; i < b+w; i++ {
		cow.write(i, aHashes[counter])
		counter++
	}
}

func (cow *cowForest) size() uint64 {
	return uint64((2 << cow.manifest.forestRows) - 1)
}

func (cow *cowForest) resize(newSize uint64) {
	cow.manifest.forestRows = treeRows((newSize + 1) >> 1)

	// How many treeBlockRows are needed to represent the current forest?
	treeBlockRowCount := cow.manifest.forestRows / rowPerTreeBlock

	// Check if there are already treeTables && location for this
	// treeBlockRow. If not, append one
	if len(cow.manifest.location) <= int(treeBlockRowCount) {
		cow.manifest.location = append(cow.manifest.location, []uint64{})
		cow.newTable(treeBlockRowCount)
	}

	// append new treeTables as needed
	for row := uint8(0); row <= treeBlockRowCount; row++ {
		currentCap := len(cow.manifest.location[row]) * nodesPerTreeTable
		// only add new tables if the current row can't hold what's needed
		for newSize > uint64(currentCap) {
			cow.newTable(row)
			currentCap += nodesPerTreeTable
		}

		// size for the next row
		newSize >>= treeBlockRows
	}

}

func (cow *cowForest) close() {
	// commit the current forest
	err := cow.commit()
	if err != nil {
		fmt.Printf("cowForest close error:\n%s\n"+
			"Previously saved forest not overwritten", err)
	}

	// clean up all old stale files
	err = cow.clean()
	if err != nil {
		panic(err)
	}
}

// Adds a single new table to the given treeBlockRow in memory
func (cow *cowForest) newTable(treeBlockRow uint8) {
	// check if there needs to be a flush
	if len(cow.cachedTreeTables)+1 > cow.meta.maxCachedTreeTables {
		cow.flush()
	}

	if len(cow.manifest.location) <= int(treeBlockRow) {
		cow.manifest.location = append(cow.manifest.location, []uint64{})
	}
	cow.manifest.fileNum++
	cow.manifest.location[treeBlockRow] = append(
		cow.manifest.location[treeBlockRow], cow.manifest.fileNum)

	newTable := newTreeTable()

	cow.cachedTreeTables[cow.manifest.fileNum] = newTable
}

// Update the cowForest num given table location. Returns the new location
func (cow *cowForest) updateTableNum(
	location, treeBlockOffset uint64, treeBlockRow uint8) uint64 {
	// grab the table
	table := cow.cachedTreeTables[location]
	if table == nil {
		panic("fetched table is nil")
	}
	tableNew := *table

	treeTableOffset := treeBlockOffset / treeBlockPerTable

	// advance fileNum and set as new file
	cow.manifest.fileNum++
	cow.manifest.location[treeBlockRow][treeTableOffset] =
		cow.manifest.fileNum

	// set as table
	cow.cachedTreeTables[cow.manifest.fileNum] = &tableNew

	// delete old key
	delete(cow.cachedTreeTables, location)

	t := cow.cachedTreeTables[cow.manifest.fileNum]
	if t == nil {
		panic("fetched t is nil")
	}

	return cow.manifest.fileNum
}

// Load will load the existing forest from the disk given a fileNumber
func (cow *cowForest) load(fileNum uint64) error {
	// check if there needs to be a flush
	// +1 to include for the requested treeTable to be loaded
	if len(cow.cachedTreeTables)+1 > cow.meta.maxCachedTreeTables {
		cow.flush()
	}

	stringLoc := strconv.FormatUint(fileNum, 10) // base 10 used
	fName := filepath.Join(cow.meta.fBasePath, stringLoc+extension)

	if verbose {
		fmt.Println("FILE LOADED: ", fName)
	}
	f, err := os.Open(fName)
	if err != nil {
		// If the error returned is of no files existing, then the manifest
		// is corrupt
		if os.IsNotExist(err) {
			// TODO Not sure if we can recover from this? I think panic
			// is the right call
			panic(errorCorruptManifest())

		}
		return err
	}
	// 2 bytes for metadata
	buf := bufio.NewReaderSize(f, (treeBlockPerTable*nodesPerTreeBlock*leafSize)+2)

	lenBytes := make([]byte, 2)

	_, err = buf.Read(lenBytes)
	if err != nil {
		return err
	}

	treeBlockCount := binary.LittleEndian.Uint16(lenBytes)
	if verbose {
		fmt.Printf("LOAD for file:%d, treeBlockCount: %d\n", fileNum, treeBlockCount)
	}

	tt, err := deserializeTreeTable(buf, treeBlockCount)
	if err != nil {
		return err
	}

	// set map
	cow.cachedTreeTables[fileNum] = tt

	return nil
}

func (cow *cowForest) flush() error {
	// commit current forest
	err := cow.commit()
	if err != nil {
		fmt.Printf("cowForest close error:\n%s\n"+
			"Previously saved forest not overwritten", err)
	}

	err = cow.clean()
	if err != nil {
		panic(err)
	}

	// replace current map with new map
	cow.cachedTreeTables = make(map[uint64]*treeTable)

	// replace manifest with the new one
	newMani := new(manifest)

	err = newMani.load(cow.meta.fBasePath)
	if err != nil {
		return err
	}

	cow.manifest = *newMani

	return nil
}

// commit makes writes to the disk and sets the forest to point to the new
// treeBlocks. The new forest state is commited to disk only when commit is called
func (cow *cowForest) commit() error {
	for fileNum, treeTable := range cow.cachedTreeTables {
		treeBlockCount, serializedTable := treeTable.serialize()

		lenBytes := make([]byte, 2)
		binary.LittleEndian.PutUint16(lenBytes[:], treeBlockCount)

		if verbose {
			fmt.Printf("COMMIT for file:%d, treeBlockCount: %d\n",
				fileNum, treeBlockCount)
			fmt.Printf("Size of serializedTable: %d\n", len(serializedTable))
		}
		// write to that file. First buffer it
		var bytesToWrite bytes.Buffer

		_, err := bytesToWrite.Write(lenBytes)
		if err != nil {
			return err
		}
		_, err = bytesToWrite.Write(serializedTable)
		if err != nil {
			return err
		}

		// actual writing to file
		// calculate the file name
		stringLoc := strconv.FormatUint(fileNum, 10) // base 10 used
		fPath := filepath.Join(cow.meta.fBasePath, stringLoc)
		fName := fPath + extension

		f, err := os.OpenFile(fName, os.O_CREATE|os.O_RDWR, 0666)
		if err != nil {
			return err
		}
		_, err = f.Write(bytesToWrite.Bytes())
		if err != nil {
			return err
		}
	}

	err := cow.manifest.commit(cow.meta.fBasePath)
	if err != nil {
		// maybe if it couldn't commit then it should panic?
		return err
	}

	return nil
}

// Clean removes all the stale treeTables from the disk and resets staleFiles field
func (cow *cowForest) clean() error {
	for _, fileNum := range cow.meta.staleFiles {
		if verbose {
			fmt.Printf("CLEANING UP file %d\n", fileNum)
		}
		stringLoc := strconv.FormatUint(fileNum, 10) // base 10 used
		filePath := filepath.Join(cow.meta.fBasePath, stringLoc+extension)
		err := os.Remove(filePath)
		if err != nil {
			return err
		}
	}

	// empty staleFiles
	cow.meta.staleFiles = []uint64{}

	return nil
}

type diskForestData struct {
	file *os.File
}

// read ignores errors. Probably get an empty hash if it doesn't work
func (d *diskForestData) read(pos uint64) Hash {
	var h Hash
	_, err := d.file.ReadAt(h[:], int64(pos*leafSize))
	if err != nil {
		fmt.Printf("\tWARNING!! read %x pos %d %s\n", h, pos, err.Error())
	}
	return h
}

// writeHash writes a hash.  Don't go out of bounds.
func (d *diskForestData) write(pos uint64, h Hash) {
	_, err := d.file.WriteAt(h[:], int64(pos*leafSize))
	if err != nil {
		fmt.Printf("\tWARNING!! write pos %d %s\n", pos, err.Error())
	}
}

// swapHash swaps 2 hashes.  Don't go out of bounds.
func (d *diskForestData) swapHash(a, b uint64) {
	ha := d.read(a)
	hb := d.read(b)
	d.write(a, hb)
	d.write(b, ha)
}

// swapHashRange swaps 2 continuous ranges of hashes.  Don't go out of bounds.
// uses lots of ram to make only 3 disk seeks (depending on how you count? 4?)
// seek to a start, read a, seek to b start, read b, write b, seek to a, write a
// depends if you count seeking from b-end to b-start as a seek. or if you have
// like read & replace as one operation or something.
func (d *diskForestData) swapHashRange(a, b, w uint64) {
	arange := make([]byte, leafSize*w)
	brange := make([]byte, leafSize*w)
	_, err := d.file.ReadAt(arange, int64(a*leafSize)) // read at a
	if err != nil {
		fmt.Printf("\tshr WARNING!! read pos %d len %d %s\n",
			a*leafSize, w, err.Error())
	}
	_, err = d.file.ReadAt(brange, int64(b*leafSize)) // read at b
	if err != nil {
		fmt.Printf("\tshr WARNING!! read pos %d len %d %s\n",
			b*leafSize, w, err.Error())
	}
	_, err = d.file.WriteAt(arange, int64(b*leafSize)) // write arange to b
	if err != nil {
		fmt.Printf("\tshr WARNING!! write pos %d len %d %s\n",
			b*leafSize, w, err.Error())
	}
	_, err = d.file.WriteAt(brange, int64(a*leafSize)) // write brange to a
	if err != nil {
		fmt.Printf("\tshr WARNING!! write pos %d len %d %s\n",
			a*leafSize, w, err.Error())
	}
}

// size gives you the size of the forest
func (d *diskForestData) size() uint64 {
	s, err := d.file.Stat()
	if err != nil {
		fmt.Printf("\tWARNING: %s. Returning 0", err.Error())
		return 0
	}
	return uint64(s.Size() / leafSize)
}

// resize makes the forest bigger (never gets smaller so don't try)
func (d *diskForestData) resize(newSize uint64) {
	err := d.file.Truncate(int64(newSize * leafSize * 2))
	if err != nil {
		panic(err)
	}
}

func (d *diskForestData) close() {
	err := d.file.Close()
	if err != nil {
		fmt.Printf("diskForestData close error: %s\n", err.Error())
	}
}
