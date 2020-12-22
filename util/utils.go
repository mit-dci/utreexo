package util

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"os"
	"sort"
	"time"

	"github.com/btcsuite/btcutil"

	"github.com/btcsuite/btcd/chaincfg"
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
func GenHashForNet(p chaincfg.Params) (*Hash, error) {

	switch p.Name {
	case "testnet3":
		return &testNet3GenHash, nil
	case "mainnet":
		return &mainNetGenHash, nil
	case "regtest":
		return &regTestGenHash, nil
	}
	return nil, fmt.Errorf("net not supported")
}

// UblockNetworkReader gets Ublocks from the remote host and puts em in the
// channel.  It'll try to fill the channel buffer.
func UblockNetworkReader(
	blockChan chan UBlockWithSkiplists, remoteServer string,
	curHeight, lookahead int32) {

	d := net.Dialer{Timeout: 2 * time.Second}
	con, err := d.Dial("tcp", remoteServer)
	if err != nil {
		panic(err)
	}
	defer con.Close()
	defer close(blockChan)

	var ub UBlockWithSkiplists
	// var ublen uint32
	// request range from curHeight to latest block
	err = binary.Write(con, binary.BigEndian, curHeight)
	if err != nil {
		fmt.Printf("write error to connection %s %s\n",
			con.RemoteAddr().String(), err.Error())
		panic("UblockNetworkReader")
		return
	}
	err = binary.Write(con, binary.BigEndian, int32(math.MaxInt32))
	if err != nil {
		fmt.Printf("write error to connection %s %s\n",
			con.RemoteAddr().String(), err.Error())
		panic("UblockNetworkReader")
		return
	}

	// TODO goroutines for only the Deserialize part might be nice.
	// Need to sort the blocks though if you're doing that
	for ; ; curHeight++ {

		err = ub.DeserializeCompact(con)
		if err != nil {
			fmt.Printf("Deserialize error from connection %s %s\n",
				con.RemoteAddr().String(), err.Error())
			// panic("UblockNetworkReader")
			return
		}

		// fmt.Printf("got ublock h %d, total size %d %d block %d udata\n",
		// 	ub.UtreexoData.Height, ub.SerializeSize(),
		// 	ub.Block.SerializeSize(), ub.UtreexoData.SerializeSize())

		blockChan <- ub
	}
}

// turns an outpoint into a 36 byte... mixed endian thing.
// (the 32 bytes txid is "reversed" and the 4 byte index is in order (big)
func OutpointToBytes(op *wire.OutPoint) (b [36]byte) {
	copy(b[0:32], op.Hash[:])
	binary.BigEndian.PutUint32(b[32:36], op.Index)
	return
}

// BlockToAdds turns all the new utxos in a msgblock into leafTxos
// uses remember slice up to number of txos, but doesn't check that it's the
// right length.  Similar with skiplist, doesn't check it.
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

// blockToDelOPs returns all the outpoints in a block that need proofs in order
// to be deleted.  All txinputs except for the coinbase input and utxos created
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
			i++ // coinbase has 1 input
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
			i += uint32(len(tx.TxOut)) // coinbase can have multiple outputs
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

// it'd be cool if you just had .sort() methods on slices of builtin types...
func sortUint16s(s []uint16) {
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
		return nil, nil, fmt.Errorf("Prefixed %d but payload %d left", l, len(payload))
	}
	return payload[:l], payload[l:], nil
}

// CheckMagicByte checks for the Bitcoin magic bytes.
// returns false if it didn't read the Bitcoin magic bytes.
// Checks only for testnet3 and mainnet
func CheckMagicByte(bytesgiven []byte) bool {
	if bytes.Compare(bytesgiven, []byte{0x0b, 0x11, 0x09, 0x07}) != 0 && //testnet
		bytes.Compare(bytesgiven, []byte{0xf9, 0xbe, 0xb4, 0xd9}) != 0 && // mainnet
		bytes.Compare(bytesgiven, []byte{0xfa, 0xbf, 0xb5, 0xda}) != 0 { // regtest
		fmt.Printf("got non magic bytes %x, finishing\n", bytesgiven)
		return false
	}

	return true
}

// HasAccess reports whether we have access to the named file.
// Returns true if HasAccess, false if it doesn't.
// Does NOT tell us if the file exists or not.
// File might exist but may not be available to us
func HasAccess(fileName string) bool {
	_, err := os.Stat(fileName)
	if err != nil && os.IsNotExist(err) {
		return false
	}
	return true
}

// IsUnspendable determines whether a txout is spendable or not.
// returns true if spendable, false if unspendable.
func IsUnspendable(o *wire.TxOut) bool {
	switch {
	case len(o.PkScript) > 10000: //len 0 is OK, spendable
		return true
	case len(o.PkScript) > 0 && o.PkScript[0] == 0x6a: // OP_RETURN is 0x6a
		return true
	default:
		return false
	}
}

// Returns true for p2pkh outputs by checking the opcode bytes
func IsP2PKH(pks []byte) bool {
	return len(pks) == 25 &&
		pks[0] == 0x76 && pks[1] == 0xa9 && pks[2] == 0x14 &&
		pks[23] == 0x88 && pks[24] == 0xac
}

// given a P2PKH scriptSig, output the original scriptPubKey
func RecoverPkScriptP2PKH(scriptSig []byte) ([]byte, error) {
	if len(scriptSig) == 0 {
		return nil, fmt.Errorf("RecoverPkScriptP2PKH given empty scriptSig")
	}
	siglen := scriptSig[0]
	if len(scriptSig)+1 < int(siglen) {
		return nil, fmt.Errorf("RecoverPkScriptP2PKH can't pop signature")
	}
	scriptSig = scriptSig[siglen+1:]
	pklen := scriptSig[0]
	if len(scriptSig)+1 < int(pklen) {
		return nil, fmt.Errorf("RecoverPkScriptP2PKH can't pop pubkey")
	}
	pkh := btcutil.Hash160(scriptSig[1 : 1+pklen])
	return p2pkhify(pkh), nil
}

// turns a pubkey hash into a normal bitcoin p2pkh spend script.
// no checks on the input.  if it's not 20 bytes, will return a script
// that won't work.
func p2pkhify(pkh []byte) (script []byte) {
	script = []byte{0x76, 0xa9, 0x14}
	script = append(script, pkh...)
	script = append(script, []byte{0x88, 0xac}...)
	return
}
