package accumulator

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// BatchProof :
type BatchProof struct {
	Targets []uint64
	Proof   []Hash
	// list of leaf locations to delete, along with a bunch of hashes that give the proof.
	// the position of the hashes is implied / computable from the leaf positions
}

// ToBytes give the bytes for a BatchProof.  It errors out silently because
// I don't think the binary.Write errors ever actually happen
func (bp *BatchProof) ToBytes() []byte {
	var buf bytes.Buffer

	// first write the number of targets (4 byte uint32)
	numTargets := uint32(len(bp.Targets))
	if numTargets == 0 {
		return nil
	}
	err := binary.Write(&buf, binary.BigEndian, numTargets)
	if err != nil {
		fmt.Printf("huh %s\n", err.Error())
		return nil
	}
	for _, t := range bp.Targets {
		// there's no need for these to be 64 bit for the next few decades...
		err := binary.Write(&buf, binary.BigEndian, t)
		if err != nil {
			fmt.Printf("huh %s\n", err.Error())
			return nil
		}
	}
	// then the rest is just hashes
	for _, h := range bp.Proof {
		_, err = buf.Write(h[:])
		if err != nil {
			fmt.Printf("huh %s\n", err.Error())
			return nil
		}
	}

	return buf.Bytes()
}

// ToString for debugging, shows the blockproof
func (bp *BatchProof) SortTargets() {
	sortUint64s(bp.Targets)
}

// ToString for debugging, shows the blockproof
func (bp *BatchProof) ToString() string {
	s := fmt.Sprintf("%d targets: ", len(bp.Targets))
	for _, t := range bp.Targets {
		s += fmt.Sprintf("%d\t", t)
	}
	s += fmt.Sprintf("\n%d proofs: ", len(bp.Proof))
	for _, p := range bp.Proof {
		s += fmt.Sprintf("%04x\t", p[:4])
	}
	s += "\n"
	return s
}

// FromBytesBatchProof gives a block proof back from the serialized bytes
func FromBytesBatchProof(b []byte) (BatchProof, error) {
	var bp BatchProof

	// if empty slice, return empty BatchProof with 0 targets
	if len(b) == 0 {
		return bp, nil
	}
	// otherwise, if there are less than 4 bytes we can't even see the number
	// of targets so something is wrong
	if len(b) < 4 {
		return bp, fmt.Errorf("batchproof only %d bytes", len(b))
	}

	buf := bytes.NewBuffer(b)
	// read 4 byte number of targets
	var numTargets uint32
	err := binary.Read(buf, binary.BigEndian, &numTargets)
	if err != nil {
		return bp, err
	}
	bp.Targets = make([]uint64, numTargets)
	for i, _ := range bp.Targets {
		err := binary.Read(buf, binary.BigEndian, &bp.Targets[i])
		if err != nil {
			return bp, err
		}
	}
	remaining := buf.Len()
	// the rest is hashes
	if remaining%32 != 0 {
		return bp, fmt.Errorf("%d bytes left, should be n*32", buf.Len())
	}
	bp.Proof = make([]Hash, remaining/32)

	for i, _ := range bp.Proof {
		copy(bp.Proof[i][:], buf.Next(32))
	}
	return bp, nil
}

// TODO OH WAIT -- this is not how to to it!  Don't hash all the way up to the
// roots to verify -- just hash up to any populated node!  Saves a ton of CPU!

// verifyBatchProof takes a block proof and reconstructs / verifies it.
// takes a blockproof to verify, and the known correct roots to check against.
// also takes the number of leaves and forest rows (those are redundant
// if we don't do weird stuff with overly-high forests, which we might)
// it returns a bool of whether the proof worked, and a map of the sparse
// forest in the blockproof
func verifyBatchProof(
	bp BatchProof, roots []Hash,
	numLeaves uint64, rows uint8) (bool, map[uint64]Hash) {

	// if nothing to prove, it worked
	if len(bp.Targets) == 0 {
		return true, nil
	}

	// Construct a map with positions to hashes
	proofmap, err := bp.Reconstruct(numLeaves, rows)
	if err != nil {
		fmt.Printf("VerifyBlockProof Reconstruct ERROR %s\n", err.Error())
		return false, proofmap
	}

	rootPositions, rootRows := getRootsReverse(numLeaves, rows)

	// partial forest is built, go through and hash everything to make sure
	// you get the right roots

	tagRow := bp.Targets
	nextRow := []uint64{}
	sortUint64s(tagRow) // probably don't need to sort

	// TODO it's ugly that I keep treating the 0-row as a special case,
	// and has led to a number of bugs.  It *is* special in a way, in that
	// the bottom row is the only thing you actually prove and add/delete,
	// but it'd be nice if it could all be treated uniformly.

	if verbose {
		fmt.Printf("tagrow len %d\n", len(tagRow))
	}

	var left, right uint64

	// iterate through rows
	for row := uint8(0); row <= rows; row++ {
		// iterate through tagged positions in this row
		for len(tagRow) > 0 {
			// Efficiency gains here. If there are two or more things to verify,
			// check if the next thing to verify is the sibling of the current leaf
			// we're on. Siblingness can be checked with bitwise XOR but since targets are
			// sorted, we can do bitwise OR instead.
			if len(tagRow) > 1 && tagRow[0]|1 == tagRow[1] {
				left = tagRow[0]
				right = tagRow[1]
				tagRow = tagRow[2:]
			} else { // if not only use one tagged position
				right = tagRow[0] | 1
				left = right ^ 1
				tagRow = tagRow[1:]
			}

			if verbose {
				fmt.Printf("left %d rootPoss %d\n", left, rootPositions[0])
			}
			// check for roots
			if left == rootPositions[0] {
				if verbose {
					fmt.Printf("one left in tagrow; should be root\n")
				}
				// Grab the hash of this position from the map
				computedRootHash, ok := proofmap[left]
				if !ok {
					fmt.Printf("ERR no proofmap for root at %d\n", left)
					return false, nil
				}
				// Verify that this root hash matches the one we stored
				if computedRootHash != roots[0] {
					fmt.Printf("row %d root, pos %d expect %04x got %04x\n",
						row, left, roots[0][:4], computedRootHash[:4])
					return false, nil
				}
				// otherwise OK and pop of the root
				roots = roots[1:]
				rootPositions = rootPositions[1:]
				rootRows = rootRows[1:]
				break
			}

			// Grab the parent position of the leaf we've verified
			parentPos := parent(left, rows)
			if verbose {
				fmt.Printf("%d %04x %d %04x -> %d\n",
					left, proofmap[left], right, proofmap[right], parentPos)
			}

			// this will crash if either is 0000
			// reconstruct the next row and add the parent to the map
			parhash := parentHash(proofmap[left], proofmap[right])
			nextRow = append(nextRow, parentPos)
			proofmap[parentPos] = parhash
		}

		// Make the nextRow the tagRow so we'll be iterating over it
		// reset th nextRow
		tagRow = nextRow
		nextRow = []uint64{}

		// if done with row and there's a root left on this row, remove it
		if len(rootRows) > 0 && rootRows[0] == row {
			// bit ugly to do these all separately eh
			roots = roots[1:]
			rootPositions = rootPositions[1:]
			rootRows = rootRows[1:]
		}
	}

	return true, proofmap
}

// Reconstruct takes a number of leaves and rows, and turns a block proof back
// into a partial proof tree. Should leave bp intact
func (bp *BatchProof) Reconstruct(
	numleaves uint64, forestRows uint8) (map[uint64]Hash, error) {

	if verbose {
		fmt.Printf("reconstruct blockproof %d tgts %d hashes nl %d fr %d\n",
			len(bp.Targets), len(bp.Proof), numleaves, forestRows)
	}
	proofTree := make(map[uint64]Hash)

	// If there is nothing to reconstruct, return empty map
	if len(bp.Targets) == 0 {
		return proofTree, nil
	}
	proof := bp.Proof // back up proof
	targets := bp.Targets
	rootPositions, rootRows := getRootsReverse(numleaves, forestRows)

	if verbose {
		fmt.Printf("%d roots:\t", len(rootPositions))
		for _, t := range rootPositions {
			fmt.Printf("%d ", t)
		}
	}
	// needRow / nextrow hold the positions of the data which should be in the blockproof
	var needSibRow, nextRow []uint64 // only even siblings needed

	// a bit strange; pop off 2 hashes at a time, and either 1 or 2 positions
	for len(bp.Proof) > 0 && len(targets) > 0 {

		if targets[0] == rootPositions[0] {
			// target is a root; this can only happen at row 0;
			// there's a "proof" but don't need to actually send it
			if verbose {
				fmt.Printf("placed single proof at %d\n", targets[0])
			}
			proofTree[targets[0]] = bp.Proof[0]
			bp.Proof = bp.Proof[1:]
			targets = targets[1:]
			continue
		}

		// there should be 2 proofs left then
		if len(bp.Proof) < 2 {
			return nil, fmt.Errorf("only 1 proof left but need 2 for %d",
				targets[0])
		}

		// populate first 2 proof hashes
		right := targets[0] | 1
		left := right ^ 1

		proofTree[left] = bp.Proof[0]
		proofTree[right] = bp.Proof[1]
		needSibRow = append(needSibRow, parent(targets[0], forestRows))
		// pop em off
		if verbose {
			fmt.Printf("placed proofs at %d, %d\n", left, right)
		}
		bp.Proof = bp.Proof[2:]

		if len(targets) > 1 && targets[0]|1 == targets[1] {
			// pop off 2 positions
			targets = targets[2:]
		} else {
			// only pop off 1
			targets = targets[1:]
		}
	}

	// deal with 0-row root, regardless of whether it was used or not
	if rootRows[0] == 0 {
		rootPositions = rootPositions[1:]
		rootRows = rootRows[1:]
	}

	// now all that's left is the proofs. go bottom to root and iterate the haveRow
	for h := uint8(1); h < forestRows; h++ {
		for len(needSibRow) > 0 {
			// if this is a root, it's not needed or given
			if needSibRow[0] == rootPositions[0] {
				needSibRow = needSibRow[1:]
				rootPositions = rootPositions[1:]
				rootRows = rootRows[1:]
				continue
			}
			// either way, we'll get the parent
			nextRow = append(nextRow, parent(needSibRow[0], forestRows))

			// if we have both siblings here, don't need any proof
			if len(needSibRow) > 1 && needSibRow[0]^1 == needSibRow[1] {
				needSibRow = needSibRow[2:]
			} else {
				// return error if we need a proof and can't get it
				if len(bp.Proof) == 0 {
					fmt.Printf("roots %v needsibrow %v\n", rootPositions, needSibRow)
					return nil, fmt.Errorf("h %d no proofs left at pos %d ",
						h, needSibRow[0]^1)
				}
				// otherwise we do need proof; place in sibling position and pop off
				proofTree[needSibRow[0]^1] = bp.Proof[0]
				bp.Proof = bp.Proof[1:]
				// and get rid of 1 element of needSibRow
				needSibRow = needSibRow[1:]
			}
		}

		// there could be a root on this row that we don't need / use; if so pop it
		if len(rootRows) > 0 && rootRows[0] == h {
			rootPositions = rootPositions[1:]
			rootRows = rootRows[1:]
		}

		needSibRow = nextRow
		nextRow = []uint64{}
	}
	if len(bp.Proof) != 0 {
		return nil, fmt.Errorf("too many proofs, %d remain", len(bp.Proof))
	}
	bp.Proof = proof // restore from backup
	return proofTree, nil
}
