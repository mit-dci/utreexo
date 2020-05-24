package blockchain

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/btcsuite/btcd/wire"
	"github.com/mit-dci/utreexo/accumulator"
	"github.com/mit-dci/utreexo/leaftx"
	"github.com/mit-dci/utreexo/util"
)

// CSNBlock includes everything a Utreexo CSN node needs to validate.
type CSNBlock struct {
	Height       int32        // Height of the block
	Transactions []*leaftx.Tx // Txs in the block
	AccData      UData        // Utreexo accumulator inclusion proofs
}

// ProofsProveBlock checks the consistency of a UBlock. Does the proof prove
// all the inputs in the block?
func (b *CSNBlock) ProofsProveBlock(inputSkipList []uint32) bool {
	// get the outpoints that need proof
	proveOPs := blockToDelOPs(b, inputSkipList)

	// ensure that all outpoints are provided in the extradata
	if len(proveOPs) != len(b.AccData.UtxoData) {
		fmt.Printf("%d outpoints need proofs but only %d proven\n",
			len(proveOPs), len(b.AccData.UtxoData))
		return false
	}
	for i, _ := range b.AccData.UtxoData {
		if proveOPs[i] != b.AccData.UtxoData[i].Outpoint {
			fmt.Printf("block/utxoData mismatch %s v %s\n",
				proveOPs[i].String(), b.AccData.UtxoData[i].Outpoint.String())
			return false
		}
	}
	return true
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
	util.SortUint32s(inskip)
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
			if util.IsUnspendable(out) {
				txonum++
				continue
			}
			// Skip txos on the skip list
			if len(skiplist) > 0 && skiplist[0] == txonum {
				skiplist = skiplist[1:]
				txonum++
				continue
			}

			var l leaftx.LeafData
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
	blk *CSNBlock, skiplist []uint32) (delOPs []wire.OutPoint) {

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

			delOPs = append(delOPs, txin.WireTxIn.PreviousOutPoint)
			blockInIdx++
		}
	}
	return
}

// UData is for "Utreexo Data". It is the accumulator proofs for
// Utreexo trees per a Bitcoin block.
type UData struct {
	AccProof       accumulator.BatchProof // Accumulator Proof for Utreexo validation
	UtxoData       []leaftx.LeafData      // Tx validation data. This is what is hashed
	RememberLeaves []bool                 // RememberLeaves is used for caching
}

// genBlockProof calls forest.ProveBatch with the hash data to get a batched
// inclusion proof from the accumulator. It then adds on the utxo leaf data,
// to create a block proof which both proves inclusion and gives all utxo data
// needed for transaction verification.
func GenUData(delLeaves []leaftx.LeafData, f *accumulator.Forest, height int32) (
	ud UData, err error) {

	ud.UtxoData = delLeaves
	// make slice of hashes from leafdata
	delHashes := make([]accumulator.Hash, len(ud.UtxoData))
	for i, _ := range ud.UtxoData {
		delHashes[i] = ud.UtxoData[i].LeafHash()
		// fmt.Printf("del %s -> %x\n",
		// ud.UtxoData[i].Outpoint.String(), delHashes[i][:4])
	}
	// generate block proof. Errors if the tx cannot be proven
	// Should never error out with genproofs as it takes
	// blk*.dat files which have already been vetted by Bitcoin Core
	ud.AccProof, err = f.ProveBatch(delHashes)
	if err != nil {
		err = fmt.Errorf("genUData failed at block %d %s %s",
			height, f.Stats(), err.Error())
		return
	}

	if len(ud.AccProof.Targets) != len(delLeaves) {
		err = fmt.Errorf("genUData %d targets but %d leafData",
			len(ud.AccProof.Targets), len(delLeaves))
		return
	}

	// fmt.Printf(batchProof.ToString())
	// Optional Sanity check. Should never fail.

	// unsort := make([]uint64, len(ud.AccProof.Targets))
	// copy(unsort, ud.AccProof.Targets)
	// ud.AccProof.SortTargets()
	// ok := f.VerifyBatchProof(ud.AccProof)
	// if !ok {
	// 	return ud, fmt.Errorf("VerifyBatchProof failed at block %d", height)
	// }
	// ud.AccProof.Targets = unsort

	// also optional, no reason to do this other than bug checking

	// if !ud.Verify(f.ReconstructStats()) {
	// 	err = fmt.Errorf("height %d LeafData / Proof mismatch", height)
	// 	return
	// }
	return
}

// BlockProof serialization:
// There's a bunch of variable length things (the batchProof.hashes, and the LeafDatas)
// so we prefix lengths for those.  Ordering is:
// batch proof length (4 bytes)
// batch proof
// Bunch of LeafDatas, prefixed with 2-byte lengths
func (ud *UData) ToBytes() (b []byte) {
	// first stick the batch proof on the beginning
	batchBytes := ud.AccProof.ToBytes()
	b = util.U32tB(uint32(len(batchBytes)))
	b = append(b, batchBytes...)

	// next, all the leafDatas
	for _, ld := range ud.UtxoData {
		ldb := ld.ToBytes()
		b = append(b, util.PrefixLen16(ldb)...)
	}

	return
}

// Verify checks the consistency of UData: that the utxos are proven in the
// batchproof
func (ud *UData) Verify(nl uint64, h uint8) bool {
	// this is really ugly and basically copies the whole thing to avoid
	// destroying it while verifying...

	presort := make([]uint64, len(ud.AccProof.Targets))
	copy(presort, ud.AccProof.Targets)

	ud.AccProof.SortTargets()
	mp, err := ud.AccProof.Reconstruct(nl, h)
	if err != nil {
		fmt.Printf(" Reconstruct failed %s\n", err.Error())
		return false
	}

	// make sure the udata is consistent, with the same number of leafDatas
	// as targets in the accumulator batch proof
	if len(ud.AccProof.Targets) != len(ud.UtxoData) {
		fmt.Printf("Verify failed: %d targets but %d leafdatas\n",
			len(ud.AccProof.Targets), len(ud.UtxoData))
	}

	for i, pos := range presort {
		hashInProof, exists := mp[pos]
		if !exists {
			fmt.Printf("Verify failed: Target %d not in map\n", pos)
			return false
		}
		// check if leafdata hashes to the hash in the proof at the target
		if ud.UtxoData[i].LeafHash() != hashInProof {
			fmt.Printf("Verify failed: txo %s position %d leafdata %x proof %x\n",
				ud.UtxoData[i].Outpoint.String(), pos,
				ud.UtxoData[i].LeafHash(), hashInProof)
			sib, exists := mp[pos^1]
			if exists {
				fmt.Printf("sib exists, %x\n", sib)
			}
			return false
		}
	}
	// return to presorted target list
	ud.AccProof.Targets = presort
	return true
}

func UDataFromBytes(b []byte) (ud UData, err error) {
	if len(b) < 4 {
		err = fmt.Errorf("block proof too short %d bytes", len(b))
		return
	}
	batchLen := util.BtU32(b[:4])
	if batchLen > uint32(len(b)-4) {
		err = fmt.Errorf("block proof says %d bytes but %d remain",
			batchLen, len(b)-4)
		return
	}
	b = b[4:]
	batchProofBytes := b[:batchLen]
	leafDataBytes := b[batchLen:]
	ud.AccProof, err = accumulator.FromBytesBatchProof(batchProofBytes)
	if err != nil {
		return
	}
	// got the batch proof part; now populate the leaf data part
	// first there are as many leafDatas as there are proof targets
	ud.UtxoData = make([]leaftx.LeafData, len(ud.AccProof.Targets))

	var ldb []byte
	// loop until we've filled in every leafData (or something breaks first)
	for i, _ := range ud.UtxoData {
		ldb, leafDataBytes, err = util.PopPrefixLen16(leafDataBytes)
		if err != nil {
			return
		}
		ud.UtxoData[i], err = leaftx.LeafDataFromBytes(ldb)
		if err != nil {
			return
		}
	}

	return ud, nil
}

// TODO use compact leafDatas in the block proofs -- probably 50%+ space savings
// Also should be default / the only serialization.  Whenever you've got the
// block proof, you've also got the block, so should always be OK to omit the
// data that's already in the block.

func UDataFromCompactBytes(b []byte) (UData, error) {
	var ud UData

	return ud, nil
}

func (ud *UData) ToCompactBytes() (b []byte) {
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
	offsetFile, err := os.Open(util.POffsetFilePath)
	if err != nil {
		return
	}

	proofFile, err := os.Open(util.PFilePath)
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
	// proofFile.Seek(int64(BtU32(offset[:])+8), 0)
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

// CSNNetworkReader gets Ublocks from the remote host and puts em in the
// channel.  It'll try to fill the channel buffer.
func CSNNetworkReader(
	blockChan chan CSNBlock, remoteServer string,
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
		var b CSNBlock
		err = b.Deserialize(con)
		if err != nil {
			panic(err)
		}

		b.Height = curHeight
		blockChan <- b
	}
}

// BlockAndRev is a regular block and a rev block stuck together
type BlockAndRev struct {
	Height int32
	Rev    RevBlock
	Blk    wire.MsgBlock
}

// TODO all these readers -- BlockAndRevReader, UBlockReader
// keep opening and closing files which is inefficient

// BlockReader is a wrapper around GetRawBlockFromFile so that the process
// can be made into a goroutine. As long as it's running, it keeps sending
// the entire blocktxs and height to bchan with TxToWrite type.
// It also puts in the proofs.  This will run on the archive server, and the
// data will be sent over the network to the CSN.
func BlockReader(blockChan chan BlockAndRev, maxHeight, curHeight int32) {
	for curHeight != maxHeight {
		blk, err := GetRawBlockFromFile(curHeight, util.OffsetFilePath)
		if err != nil {
			panic(err)
		}

		rb, err := GetRevBlock(curHeight, util.RevOffsetFilePath)
		if err != nil {
			panic(err)
		}

		bnr := BlockAndRev{Height: curHeight, Blk: blk, Rev: rb}

		blockChan <- bnr
		curHeight++
	}
}

// network serialization for UBlocks (regular block with udata)
// First 4 bytes is (big endian) lenght of the udata.
// Then there's just a wire.MsgBlock with the regular serialization.
// So basically udata, then a block, that's it.

// Looks like "height" doesn't get sent over this way, but maybe that's OK.
func (b *Block) Deserialize(r io.Reader) (err error) {
	err = b.Block.Deserialize(r)
	if err != nil {
		return
	}

	var uDataLen, bytesRead uint32
	var n int

	err = binary.Read(r, binary.BigEndian, &uDataLen)
	if err != nil {
		return
	}

	udataBytes := make([]byte, uDataLen)

	for bytesRead < uDataLen {
		n, err = r.Read(udataBytes[bytesRead:])
		if err != nil {
			return
		}
		bytesRead += uint32(n)
	}

	b.AccData, err = UDataFromBytes(udataBytes)
	return
}

func (b *Block) Serialize(w io.Writer) (err error) {
	err = b.Block.Serialize(w)
	if err != nil {
		return
	}

	udataBytes := b.AccData.ToBytes()
	err = binary.Write(w, binary.BigEndian, uint32(len(udataBytes)))
	if err != nil {
		return
	}

	_, err = w.Write(udataBytes)

	return
}

// MakeBlock is a wrapper around GetRawBlockFromFile and GetRevBlock
// It reads both and constructs a block with all the data needed to
// validate a tx.
func MakeCSNBlock(height int32) (b CSNBlock, err error) {
	// Grab blocks
	block, err := GetRawBlockFromFile(height, util.OffsetFilePath)
	if err != nil {
		return
	}
	revBlock, err := GetRevBlock(height, util.RevOffsetFilePath)
	if err != nil {
		return
	}

	// make sure same number of txs and rev txs (minus coinbase)
	if len(block.Transactions)-1 != len(revBlock.Txs) {
		err = fmt.Errorf("genDels block %d %d txs but %d rev txs",
			height, len(block.Transactions), len(revBlock.Txs))
		return
	}

	// Grab the current block's header hash
	blockhash := block.BlockHash()

	// All the duplicate txs to skip
	// inskip for TXINs, outskip for TXOUTs
	inskip, outskip := DedupeBlock(&block)

	var blockInIdx, txonum uint32
	var txs []*leaftx.Tx
	for txinblock, tx := range block.Transactions {
		var appendtx leaftx.Tx
		if txinblock == 0 {
			blockInIdx++ // coinbase tx always has 1 input
			continue
		}
		txinblock--
		// make sure there's the same number of txins
		if len(tx.TxIn) != len(revBlock.Txs[txinblock].TxIn) {
			err = fmt.Errorf("genDels block %d tx %d has %d inputs but %d rev entries",
				height, txinblock+1,
				len(tx.TxIn), len(revBlock.Txs[txinblock].TxIn))
			return
		}
		// loop through inputs
		for i, txin := range tx.TxIn {
			// check if on inskip.  If so, don't make leaf
			if len(inskip) > 0 && inskip[0] == blockInIdx {
				// fmt.Printf("skip %s\n", txin.PreviousOutPoint.String())
				inskip = inskip[1:]
				blockInIdx++
				continue
			}

			var leafIn leaftx.TxIn
			// TODO don't allocate blockhash here in the future. You only need one per block
			leafIn.ValData.BlockHash = blockhash
			leafIn.ValData.Outpoint = txin.PreviousOutPoint
			leafIn.ValData.Height = revBlock.Txs[txinblock].TxIn[i].Height
			leafIn.ValData.Coinbase = revBlock.Txs[txinblock].TxIn[i].Coinbase
			leafIn.ValData.Amt = revBlock.Txs[txinblock].TxIn[i].Amount
			leafIn.ValData.PkScript = revBlock.Txs[txinblock].TxIn[i].PKScript
			blockInIdx++

			// Append TxIn
			appendtx.TxIn = append(appendtx.TxIn, &leafIn)
		}

		/*
			// cache txid aka txhash
			txid := tx.TxHash()
			for i, out := range tx.TxOut {
				// Skip all the OP_RETURNs
				if util.IsUnspendable(out) {
					txonum++
					continue
				}
				// Skip txos on the skip list
				if len(outskip) > 0 && outskip[0] == txonum {
					outskip = outskip[1:]
					txonum++
					continue
				}

				var leafout leaftx.TxOut
				leafout.ValData.BlockHash = blockhash
				leafout.ValData.Outpoint.Hash = txid
				leafout.ValData.Outpoint.Index = uint32(i)
				leafout.ValData.Height = height
				if txinblock == 0 {
					leafout.ValData.Coinbase = true
				}
				leafout.ValData.Amt = out.Value
				leafout.ValData.PkScript = out.PkScript
				leafout.Leaf = accumulator.Leaf{Hash: leafout.ValData.LeafHash()}
				// TODO init remember
				/*
					if uint32(len(remember)) > txonum {
						uleaf.Remember = remember[txonum]
					}
				appendtx.TxOut = append(appendtx.TxOut, &leafout)
				txonum++
			}
		*/
	}
	b.Transactions = txs
	b.Height = height

	//ud, err := genUData()

	return
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

/*
func constructTx(inskip, outskip []uint32, height int32, remember []bool,
	hash chainhash.Hash, block *wire.MsgBlock, revBlock *RevBlock) (Transaction leaftx.Tx, err error) {

	var blockInIdx uint32
	var txonum uint32
	for txinblock, tx := range block.Transactions {
		var TxIns []*leaftx.TxIn
		if txinblock == 0 {
			blockInIdx++ // coinbase tx always has 1 input
			continue
		}
		txinblock--
		// make sure there's the same number of txins
		if len(tx.TxIn) != len(revBlock.Txs[txinblock].TxIn) {
			err = fmt.Errorf("genDels block %d tx %d has %d inputs but %d rev entries",
				height, txinblock+1,
				len(tx.TxIn), len(revBlock.Txs[txinblock].TxIn))
			return
		}
		// loop through inputs
		for i, txin := range tx.TxIn {
			// check if on inskip.  If so, don't make leaf
			if len(inskip) > 0 && inskip[0] == blockInIdx {
				// fmt.Printf("skip %s\n", txin.PreviousOutPoint.String())
				inskip = inskip[1:]
				blockInIdx++
				continue
			}

			var leafIn leaftx.TxIn
			// TODO don't allocate blockhash here in the future. You only need one per block
			leafIn.ValData.BlockHash = hash
			leafIn.ValData.Outpoint = txin.PreviousOutPoint
			leafIn.ValData.Height = revBlock.Txs[txinblock].TxIn[i].Height
			leafIn.ValData.Coinbase = revBlock.Txs[txinblock].TxIn[i].Coinbase
			leafIn.ValData.Amt = revBlock.Txs[txinblock].TxIn[i].Amount
			leafIn.ValData.PkScript = revBlock.Txs[txinblock].TxIn[i].PKScript
			blockInIdx++

			TxIns = append(TxIns, &leafIn)
		}

		var leaves []*accumulator.Leaf
		// cache txid aka txhash
		txid := tx.TxHash()
		for i, out := range tx.TxOut {
			// Skip all the OP_RETURNs
			if util.IsUnspendable(out) {
				txonum++
				continue
			}
			// Skip txos on the skip list
			if len(outskip) > 0 && outskip[0] == txonum {
				outskip = outskip[1:]
				txonum++
				continue
			}

			var l leaftx.LeafData
			l.BlockHash = hash
			l.Outpoint.Hash = txid
			l.Outpoint.Index = uint32(i)
			l.Height = height
			if txinblock == 0 {
				l.Coinbase = true
			}
			l.Amt = out.Value
			l.PkScript = out.PkScript
			uleaf := accumulator.Leaf{Hash: l.LeafHash()}
			if uint32(len(remember)) > txonum {
				uleaf.Remember = remember[txonum]
			}
			leaves = append(leaves, &uleaf)
			// fmt.Printf("add %s\n", l.ToString())
			// fmt.Printf("add %s -> %x\n", l.Outpoint.String(), l.LeafHash())
			txonum++
		}
	}

	return
}
*/

/*
// Takes in a rev and blk block to construct a TxIn
func constructTxIn(inskip []uint32, height int32, hash chainhash.Hash, block *wire.MsgBlock, revBlock *RevBlock) (
	TxIns []*leaftx.TxIn, err error) {

	for txinblock, tx := range block.Transactions {
		if txinblock == 0 {
			blockInIdx++ // coinbase tx always has 1 input
			continue
		}
		txinblock--
		// make sure there's the same number of txins
		if len(tx.TxIn) != len(revBlock.Txs[txinblock].TxIn) {
			err = fmt.Errorf("genDels block %d tx %d has %d inputs but %d rev entries",
				height, txinblock+1,
				len(tx.TxIn), len(revBlock.Txs[txinblock].TxIn))
			return
		}
		// loop through inputs
		for i, txin := range tx.TxIn {
			// check if on inskip.  If so, don't make leaf
			if len(inskip) > 0 && inskip[0] == blockInIdx {
				// fmt.Printf("skip %s\n", txin.PreviousOutPoint.String())
				inskip = inskip[1:]
				blockInIdx++
				continue
			}

			var leafIn leaftx.TxIn
			// TODO don't allocate blockhash here in the future. You only need one per block
			leafIn.ValData.BlockHash = hash
			leafIn.ValData.Outpoint = txin.PreviousOutPoint
			leafIn.ValData.Height = revBlock.Txs[txinblock].TxIn[i].Height
			leafIn.ValData.Coinbase = revBlock.Txs[txinblock].TxIn[i].Coinbase
			leafIn.ValData.Amt = revBlock.Txs[txinblock].TxIn[i].Amount
			leafIn.ValData.PkScript = revBlock.Txs[txinblock].TxIn[i].PKScript
			blockInIdx++

			TxIns = append(TxIns, &leafIn)
		}
	}

	return
}
*/
