package utreexo

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// BlockProof :
type BlockProof struct {
	Targets []uint64
	Proof   []Hash
	// list of leaf locations to delete, along with a bunch of hashes that give the proof.
	// the position of the hashes is implied / computable from the leaf positions
}

// ToBytes give the bytes for a blockproof.  It errors out silently because
// I don't think the binary.Write errors ever actually happen
func (bp *BlockProof) ToBytes() []byte {
	var buf bytes.Buffer

	// first write the number of targets (4 byte uint32)

	numTargets := uint32(len(bp.Targets))

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
func (bp *BlockProof) ToString() string {
	s := fmt.Sprintf("%d targets: ", len(bp.Targets))
	for _, t := range bp.Targets {
		s += fmt.Sprintf("%d\t", t)
	}
	s += fmt.Sprintf("\n%d proofs: ", len(bp.Proof))
	for _, p := range bp.Proof {
		s += fmt.Sprintf("%04x\t", p[:4])
	}
	s += fmt.Sprintf("\n")
	return s
}

// FromBytesBlockProof gives a block proof back from the serialized bytes
func FromBytesBlockProof(b []byte) (BlockProof, error) {
	var bp BlockProof

	if len(b) < 4 {
		return bp, fmt.Errorf("blockproof only %d bytes", len(b))
	}

	buf := bytes.NewBuffer(b)
	// read 4 byte number of targets
	var numTargets uint32
	err := binary.Read(buf, binary.BigEndian, &numTargets)
	if err != nil {
		return bp, err
	}
	bp.Targets = make([]uint64, numTargets)
	for i := range bp.Targets {
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

	for i := range bp.Proof {
		copy(bp.Proof[i][:], buf.Next(32))
	}
	return bp, nil
}

// TODO OH WAIT -- this is not how to to it!  Don't hash all the way up to the
// tops to verify -- just hash up to any populated node!  Saves a ton of CPU!

// VerifyBlockProof takes a block proof and reconstructs / verifies it.
// takes a blockproof to verify, and the known correct tops to check against.
// also takes the number of leaves and forest height (those are redundant
// if we don't do weird stuff with overly-high forests, which we might)
// it returns a bool of whether the proof worked, and a map of the sparse
// forest in the blockproof
func VerifyBlockProof(
	bp BlockProof, tops []Hash,
	numLeaves uint64, height uint8) (bool, map[uint64]Hash) {

	// if nothing to prove, it worked
	if len(bp.Targets) == 0 {
		return true, nil
	}

	proofmap, err := bp.Reconstruct(numLeaves, height)
	if err != nil {
		fmt.Printf("VerifyBlockProof Reconstruct ERROR %s\n", err.Error())
		return false, proofmap
	}

	//	fmt.Printf("Reconstruct complete\n")
	topposs, topheights := getTopsReverse(numLeaves, height)

	// partial forest is built, go through and hash everything to make sure
	// you get the right tops

	tagRow := bp.Targets
	nextRow := []uint64{}
	sortUint64s(tagRow) // probably don't need to sort

	// TODO it's ugly that I keep treating the 0-row as a special case,
	// and has led to a number of bugs.  It *is* special in a way, in that
	// the bottom row is the only thing you actually prove and add/delete,
	// but it'd be nice if it could all be treated uniformly.

	// if proofmap has a 0-root, check it
	if verbose {
		fmt.Printf("tagrow len %d\n", len(tagRow))
	}

	var left, right uint64
	// iterate through height

	for h := uint8(0); h < height; h++ {
		// iterate through tagged positions in this row

		for len(tagRow) > 0 {
			// see if the next tag is a sibling and we get both
			if len(tagRow) > 1 && tagRow[0]|1 == tagRow[1] {
				left = tagRow[0]
				right = tagRow[1]
				tagRow = tagRow[2:]
			} else { // if not only use one tagged position
				right = tagRow[0] | 1
				left = right ^ 1
				tagRow = tagRow[1:]
			}

			// check for tops
			if verbose {
				fmt.Printf("left %d toppos %d\n", left, topposs[0])
			}
			if left == topposs[0] {
				if verbose {
					fmt.Printf("one left in tagrow; should be top\n")
				}
				computedRoot, ok := proofmap[left]
				if !ok {
					fmt.Printf("ERR no proofmap for root at %d\n", left)
					return false, nil
				}
				if computedRoot != tops[0] {
					fmt.Printf("height %d top, pos %d expect %04x got %04x\n",
						h, left, tops[0][:4], computedRoot[:4])
					return false, nil
				}
				// otherwise OK and pop of the top
				tops = tops[1:]
				topposs = topposs[1:]
				topheights = topheights[1:]
				break
			}

			parpos := up1(left, height)
			if verbose {
				fmt.Printf("%d %04x %d %04x -> %d\n",
					left, proofmap[left], right, proofmap[right], parpos)
			}
			// this will crash if either is 0000
			parhash := Parent(proofmap[left], proofmap[right])
			nextRow = append(nextRow, parpos)
			proofmap[parpos] = parhash
		}

		tagRow = nextRow
		nextRow = []uint64{}
		// if done with row and there's a top left on this row, remove it
		if len(topheights) > 0 && topheights[0] == h {
			// bit ugly to do these all separately eh
			tops = tops[1:]
			topposs = topposs[1:]
			topheights = topheights[1:]
		}
	}

	return true, proofmap
}

// Reconstruct takes a number of leaves and height, and turns a block proof back
// into a partial proof tree.  Destroys the bp.Proofs slice but leaves the
// bp.Targets
func (bp *BlockProof) Reconstruct(
	numleaves uint64, forestHeight uint8) (map[uint64]Hash, error) {

	if verbose {
		fmt.Printf("reconstruct blockproof %d tgts %d hashes nl %d fh %d\n",
			len(bp.Targets), len(bp.Proof), numleaves, forestHeight)
	}

	proofTree := make(map[uint64]Hash)

	if len(bp.Targets) == 0 {
		return proofTree, nil
	}
	targets := bp.Targets
	tops, topheights := getTopsReverse(numleaves, forestHeight)

	//	fmt.Printf("first needrow len %d\n", len(needRow))
	if verbose {
		fmt.Printf("%d tops:\t", len(tops))
		for _, t := range tops {
			fmt.Printf("%d ", t)
		}
	}
	// needRow / nextrow hold the positions of the data which should be in the blockproof
	var needSibRow, nextRow []uint64 // only even siblings needed

	// a bit strange; pop off 2 hashes at a time, and either 1 or 2 positions
	for len(bp.Proof) > 0 && len(targets) > 0 {

		if targets[0] == tops[0] {
			// target is a top; this can only happen at row 0;
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
		needSibRow = append(needSibRow, up1(targets[0], forestHeight))
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
	if topheights[0] == 0 {
		tops = tops[1:]
		topheights = topheights[1:]
	}

	// now all that's left is the proofs. go bottom to top and iterate the haveRow
	for h := uint8(1); h < forestHeight; h++ {
		//		fmt.Printf("h %d needrow:\t", h)
		//		for _, np := range needRow {
		//			fmt.Printf(" %d", np)
		//		}
		//		fmt.Printf("\n")

		for len(needSibRow) > 0 {
			// if this is a top, it's not needed or given
			if needSibRow[0] == tops[0] {
				//				fmt.Printf("\t\tzzz pos %d is h %d top\n", needSibRow[0], h)
				needSibRow = needSibRow[1:]
				tops = tops[1:]
				topheights = topheights[1:]
				continue
			}
			// either way, we'll get the parent
			nextRow = append(nextRow, up1(needSibRow[0], forestHeight))

			// if we have both siblings here, don't need any proof
			if len(needSibRow) > 1 && needSibRow[0]^1 == needSibRow[1] {
				//				fmt.Printf("pop %d, %d\n", needRow[0], needRow[1])
				needSibRow = needSibRow[2:]
			} else {
				// return error if we need a proof and can't get it
				if len(bp.Proof) == 0 {
					fmt.Printf("tops %v needsibrow %v\n", tops, needSibRow)
					return nil, fmt.Errorf("h %d no proofs left at pos %d ",
						h, needSibRow[0]^1)
				}
				// otherwise we do need proof; place in sibling position and pop off
				proofTree[needSibRow[0]^1] = bp.Proof[0]
				bp.Proof = bp.Proof[1:]
				//				fmt.Printf("place proof at pos %d\n", needSibRow[0]^1)
				// and get rid of 1 element of needSibRow
				needSibRow = needSibRow[1:]
			}
		}

		// there could be a top at this height that we don't need / use; if so pop it
		if len(topheights) > 0 && topheights[0] == h {
			tops = tops[1:]
			topheights = topheights[1:]
		}

		needSibRow = nextRow
		nextRow = []uint64{}
	}
	if len(bp.Proof) != 0 {
		return nil, fmt.Errorf("too many proofs, %d remain", len(bp.Proof))
	}

	return proofTree, nil
}
