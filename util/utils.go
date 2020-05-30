package util

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"time"

	"github.com/btcsuite/btcd/wire"
	"github.com/mit-dci/utreexo/accumulator"
)

// Hash is just [32]byte
var mainNetGenHash = Hash{
	0x6f, 0xe2, 0x8c, 0x0a, 0xb6, 0xf1, 0xb3, 0x72,
	0xc1, 0xa6, 0xa2, 0x46, 0xae, 0x63, 0xf7, 0x4f,
	0x93, 0x1e, 0x83, 0x65, 0xe1, 0x5a, 0x08, 0x9c,
	0x68, 0xd6, 0x19, 0x00, 0x00, 0x00, 0x00, 0x00,
}

var testNet3GenHash = Hash{
	0x43, 0x49, 0x7f, 0xd7, 0xf8, 0x26, 0x95, 0x71,
	0x08, 0xf4, 0xa3, 0x0f, 0xd9, 0xce, 0xc3, 0xae,
	0xba, 0x79, 0x97, 0x20, 0x84, 0xe9, 0x0e, 0xad,
	0x01, 0xea, 0x33, 0x09, 0x00, 0x00, 0x00, 0x00,
}

var regTestGenHash = Hash{
	0x06, 0x22, 0x6e, 0x46, 0x11, 0x1a, 0x0b, 0x59,
	0xca, 0xaf, 0x12, 0x60, 0x43, 0xeb, 0x5b, 0xbf,
	0x28, 0xc3, 0x4f, 0x3a, 0x5e, 0x33, 0x2a, 0x1f,
	0xc7, 0xb2, 0xb7, 0x3c, 0xf1, 0x88, 0x91, 0x0f,
}

// For a given BitcoinNet, yields the genesis hash
// If the BitcoinNet is not supported, an error is
// returned.
func GenHashForNet(net wire.BitcoinNet) (*Hash, error) {
	switch net {
	case wire.TestNet3:
		return &testNet3GenHash, nil
	case wire.MainNet:
		return &mainNetGenHash, nil
	case wire.TestNet: // yes, this is regtest
		return &regTestGenHash, nil
	}
	return nil, fmt.Errorf("net not supported\n")
}

// Checks if the blk00000.dat file in the current directory
// is testnet3 or mainnet or regtest.
func CheckNet(net wire.BitcoinNet) {
	f, err := os.Open("blk00000.dat")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	var magicbytes [4]byte
	f.Read(magicbytes[:])

	var bytesToMatch [4]byte
	binary.LittleEndian.PutUint32(bytesToMatch[:], uint32(net))

	if bytes.Compare(magicbytes[:], bytesToMatch[:]) != 0 {
		switch net {
		case wire.TestNet3:
			fmt.Println("Option -net=testnet given but .dat file is NOT a testnet file.")
		case wire.MainNet:
			fmt.Println("Neither option -net=testnet or -net=regtest was given but .dat file is NOT a mainnet file.")
		case wire.TestNet:
			fmt.Println("Option -net=regtest given but .dat file is NOT a regtest file.")
		}
		fmt.Println("Exiting...")
		os.Exit(2)
	}
}

// TODO all these readers -- BlockAndRevReader, UBlockReader
// keep opening and closing files which is inefficient

// BlockReader is a wrapper around GetRawBlockFromFile so that the process
// can be made into a goroutine. As long as it's running, it keeps sending
// the entire blocktxs and height to bchan with TxToWrite type.
// It also puts in the proofs.  This will run on the archive server, and the
// data will be sent over the network to the CSN.
func BlockAndRevReader(
	blockChan chan BlockAndRev,
	maxHeight, curHeight int32) {
	for curHeight != maxHeight {
		blk, err := GetRawBlockFromFile(curHeight, OffsetFilePath)
		if err != nil {
			panic(err)
		}

		rb, err := GetRevBlock(curHeight, RevOffsetFilePath)
		if err != nil {
			panic(err)
		}

		bnr := BlockAndRev{Height: curHeight, Blk: blk, Rev: rb}

		blockChan <- bnr
		curHeight++
	}
}

// BlockReader is a wrapper around GetRawBlockFromFile so that the process
// can be made into a goroutine. As long as it's running, it keeps sending
// the entire blocktxs and height to bchan with TxToWrite type.
// It also puts in the proofs.  This will run on the archive server, and the
// data will be sent over the network to the CSN.
func UBlockReader(
	blockChan chan UBlock, maxHeight, curHeight, lookahead int32) {
	for curHeight != maxHeight {

		ud, err := GetUDataFromFile(curHeight)
		if err != nil {
			fmt.Printf("GetUDataFromFile ")
			panic(err)
		}

		blk, err := GetRawBlockFromFile(curHeight, OffsetFilePath)
		if err != nil {
			fmt.Printf("GetRawBlockFromFile ")
			panic(err)
		}

		send := UBlock{Block: blk, Height: curHeight, ExtraData: ud}

		blockChan <- send
		curHeight++
	}
}

// UblockNetworkReader gets Ublocks from the remote host and puts em in the
// channel.  It'll try to fill the channel buffer.
func UblockNetworkReader(
	blockChan chan UBlock, remoteServer string,
	curHeight, lookahead int32) {

	d := net.Dialer{Timeout: 2 * time.Second}
	con, err := d.Dial("tcp", "127.0.0.1:8338")
	if err != nil {
		panic(err)
	}
	defer con.Close()

	err = binary.Write(con, binary.BigEndian, curHeight)
	if err != nil {
		panic(err)
	}

	for ; ; curHeight++ {
		var ub UBlock
		err = ub.Deserialize(con)
		if err != nil {
			if err == io.EOF {
				close(blockChan)
				break
			}
			panic(err)
		}

		ub.Height = curHeight
		blockChan <- ub
	}
}

// GetRawBlocksFromFile reads the blocks from the given .dat file and
// returns those blocks.
// Skips the genesis block. If you search for block 0, it will give you
// block 1.
func GetRawBlockFromFile(tipnum int32, offsetFileName string) (
	block wire.MsgBlock, err error) {
	if tipnum == 0 {
		err = fmt.Errorf("Block 0 is not in blk files or utxo set")
		return
	}
	tipnum--

	var datFile, offset uint32

	offsetFile, err := os.Open(offsetFileName)
	if err != nil {
		return
	}

	// offset file consists of 8 bytes per block
	// tipnum * 8 gives us the correct position for that block
	_, err = offsetFile.Seek(int64(8*tipnum), 0)
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

	blockFileName := fmt.Sprintf("blk%05d.dat", datFile)
	// Channel to alert stopParse() that offset
	// fmt.Printf("opened %s ", blockFileName)
	blockFile, err := os.Open(blockFileName)
	if err != nil {
		return
	}
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

	// fmt.Printf("closed %s ", blockFileName)
	err = blockFile.Close()
	if err != nil {
		return
	}
	err = offsetFile.Close()
	if err != nil {
		return
	}

	return
}

// GetUDataFromFile reads the proof data from proof.dat and proofoffset.dat
// and gives the proof & utxo data back.
// Don't ask for block 0, there is no proof of that.
func GetUDataFromFile(tipnum int32) (ud UData, err error) {
	if tipnum == 0 {
		err = fmt.Errorf("Block 0 is not in blk files or utxo set")
		return
	}
	tipnum--
	var offset int64
	var size uint32
	offsetFile, err := os.Open(POffsetFilePath)
	if err != nil {
		return
	}

	proofFile, err := os.Open(PFilePath)
	if err != nil {
		return
	}

	// offset file consists of 8 bytes per block
	// tipnum * 8 gives us the correct position for that block
	// Note it's currently a int64, can go down to int32 for split files
	_, err = offsetFile.Seek(int64(8*tipnum), 0)
	if err != nil {
		err = fmt.Errorf("offsetFile.Seek %s", err.Error())
		return
	}

	err = binary.Read(offsetFile, binary.BigEndian, &offset)
	if err != nil {
		err = fmt.Errorf("binary.Read offset %d %s", tipnum, err.Error())
		return
	}

	// +4 because it has an empty 4 non-magic bytes in front now
	_, err = proofFile.Seek(offset+4, 0)
	if err != nil {
		err = fmt.Errorf("proofFile.Seek %s", err.Error())
		return
	}
	err = binary.Read(proofFile, binary.BigEndian, &size)
	if err != nil {
		return
	}

	// +8 skips the 8 bytes of magicbytes and load size
	// proofFile.Seek(int64(binary.BigEndian.Uint32(offset[:])+8), 0)
	ubytes := make([]byte, size)

	_, err = proofFile.Read(ubytes)
	if err != nil {
		err = fmt.Errorf("proofFile.Read(ubytes) %s", err.Error())
		return
	}

	ud, err = UDataFromBytes(ubytes)
	if err != nil {
		err = fmt.Errorf("UDataFromBytes %s", err.Error())
		return
	}

	err = offsetFile.Close()
	if err != nil {
		return
	}
	err = proofFile.Close()
	if err != nil {
		return
	}
	return
}

// BlockToAdds turns all the new utxos in a msgblock into leafTxos
// uses remember slice up to number of txos, but doesn't check that it's the
// right lenght.  Similar with skiplist, doesn't check it.
func BlockToAddLeaves(blk wire.MsgBlock,
	remember []bool, skiplist []uint32,
	height int32) (leaves []accumulator.Leaf) {

	var txonum uint32
	// bh := bl.Blockhash
	for coinbaseif0, tx := range blk.Transactions {
		// cache txid aka txhash
		txid := tx.TxHash()
		for i, out := range tx.TxOut {
			// Skip all the OP_RETURNs
			if IsUnspendable(out) {
				txonum++
				continue
			}
			// Skip txos on the skip list
			if len(skiplist) > 0 && skiplist[0] == txonum {
				skiplist = skiplist[1:]
				txonum++
				continue
			}

			var l LeafData
			// TODO put blockhash back in -- leaving empty for now!
			// l.BlockHash = bh
			l.Outpoint.Hash = txid
			l.Outpoint.Index = uint32(i)
			l.Height = height
			if coinbaseif0 == 0 {
				l.Coinbase = true
			}
			l.Amt = out.Value
			l.PkScript = out.PkScript
			uleaf := accumulator.Leaf{Hash: l.LeafHash()}
			if uint32(len(remember)) > txonum {
				uleaf.Remember = remember[txonum]
			}
			leaves = append(leaves, uleaf)
			// fmt.Printf("add %s\n", l.ToString())
			// fmt.Printf("add %s -> %x\n", l.Outpoint.String(), l.LeafHash())
			txonum++
		}
	}
	return
}

// blockToDelOPs gives all the UTXOs in a block that need proofs in order to be
// deleted.  All txinputs except for the coinbase input and utxos created
// within the same block (on the skiplist)
func blockToDelOPs(
	blk *wire.MsgBlock, skiplist []uint32) (delOPs []wire.OutPoint) {

	var blockInIdx uint32
	for txinblock, tx := range blk.Transactions {
		if txinblock == 0 {
			blockInIdx++ // coinbase tx always has 1 input
			continue
		}

		// loop through inputs
		for _, txin := range tx.TxIn {
			// check if on skiplist.  If so, don't make leaf
			if len(skiplist) > 0 && skiplist[0] == blockInIdx {
				// fmt.Printf("skip %s\n", txin.PreviousOutPoint.String())
				skiplist = skiplist[1:]
				blockInIdx++
				continue
			}

			delOPs = append(delOPs, txin.PreviousOutPoint)
			blockInIdx++
		}
	}
	return
}

// DedupeBlock takes a bitcoin block, and returns two int slices: the indexes of
// inputs, and idexes of outputs which can be removed.  These are indexes
// within the block as a whole, even the coinbase tx.
// So the coinbase tx in & output numbers affect the skip lists even though
// the coinbase ins/outs can never be deduped.  it's simpler that way.
func DedupeBlock(blk *wire.MsgBlock) (inskip []uint32, outskip []uint32) {

	var i uint32
	// wire.Outpoints are comparable with == which is nice.
	inmap := make(map[wire.OutPoint]uint32)

	// go through txs then inputs building map
	for cbif0, tx := range blk.Transactions {
		if cbif0 == 0 { // coinbase tx can't be deduped
			i++
			continue
		}
		for _, in := range tx.TxIn {
			// fmt.Printf("%s into inmap\n", in.PreviousOutPoint.String())
			inmap[in.PreviousOutPoint] = i
			i++
		}
	}

	i = 0
	// start over, go through outputs finding skips
	for cbif0, tx := range blk.Transactions {
		if cbif0 == 0 { // coinbase tx can't be deduped
			i += uint32(len(tx.TxOut))
			continue
		}
		txid := tx.TxHash()

		for outidx, _ := range tx.TxOut {
			op := wire.OutPoint{Hash: txid, Index: uint32(outidx)}
			// fmt.Printf("%s check for inmap... ", op.String())
			inpos, exists := inmap[op]
			if exists {
				// fmt.Printf("hit")
				inskip = append(inskip, inpos)
				outskip = append(outskip, i)
			}
			// fmt.Printf("\n")
			i++
		}
	}
	// sort inskip list, as it's built in order consumed not created
	sortUint32s(inskip)
	return
}

// it'd be cool if you just had .sort() methods on slices of builtin types...
func sortUint32s(s []uint32) {
	sort.Slice(s, func(a, b int) bool { return s[a] < s[b] })
}

// PrefixLen16 puts a 2 byte length prefix in front of a byte slice
func PrefixLen16(b []byte) []byte {
	l := uint16(len(b))
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, l)
	return append(buf.Bytes(), b...)
}

func PopPrefixLen16(b []byte) ([]byte, []byte, error) {
	if len(b) < 2 {
		return nil, nil, fmt.Errorf("PrefixedLen slice only %d long", len(b))
	}
	prefix, payload := b[:2], b[2:]
	var l uint16
	buf := bytes.NewBuffer(prefix)
	binary.Read(buf, binary.BigEndian, &l)
	if int(l) > len(payload) {
		return nil, nil, fmt.Errorf("Prefixed %d but payload %d left", l)
	}
	return payload[:l], payload[l:], nil
}

// CheckMagicByte checks for the Bitcoin magic bytes.
// returns false if it didn't read the Bitcoin magic bytes.
// Checks only for testnet3 and mainnet
func CheckMagicByte(bytesgiven [4]byte) bool {
	if bytesgiven != [4]byte{0x0b, 0x11, 0x09, 0x07} && //testnet
		bytesgiven != [4]byte{0xf9, 0xbe, 0xb4, 0xd9} && // mainnet
		bytesgiven != [4]byte{0xfa, 0xbf, 0xb5, 0xda} { // regtest
		fmt.Printf("got non magic bytes %x, finishing\n", bytesgiven)
		return false
	} else {
		return true
	}
}

// HasAccess reports whether we have access to the named file.
// Returns true if HasAccess, false if it doesn't.
// Does NOT tell us if the file exists or not.
// File might exist but may not be available to us
func HasAccess(fileName string) bool {
	if _, err := os.Stat(fileName); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

//IsUnspendable determines whether a tx is spenable or not.
//returns true if spendable, false if unspenable.
func IsUnspendable(o *wire.TxOut) bool {
	switch {
	case len(o.PkScript) > 10000: //len 0 is OK, spendable
		return true
	case len(o.PkScript) > 0 && o.PkScript[0] == 0x6a: //OP_RETURN is 0x6a
		return true
	default:
		return false
	}
}
