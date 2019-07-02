package utreexo

import (
	"fmt"
)

func (p *Pollard) RequestBlockProof([]uint64) ([]uint64, error) {

	return nil, nil
}

// IngestBlockProof populates the Pollard with all needed data to delete the
// targets in the block proof
func (p *Pollard) IngestBlockProof(bp BlockProof) error {
	var empty Hash
	// TODO so many things to change
	ok, proofMap := p.VerifyBlockProof(bp)
	if !ok {
		return fmt.Errorf("block proof mismatch")
	}
	//	fmt.Printf("targets: %v\n", bp.Targets)
	// go through each target and populate pollard
	for _, target := range bp.Targets {

		tNum, bits, branchLen := detectOffset(target, p.numLeaves)
		if branchLen == 0 {
			// if there's no branch (1-tree) nothing to prove
			continue
		}
		node := p.tops[tNum]
		h := branchLen - 1
		bits = ^bits                                 // flip bits for proof descent
		pos := upMany(target, branchLen, p.height()) // this works but...
		// we should have a way to get the top positions from just p.tops

		// fmt.Printf("ingest adding target %d to top %04x h %d brlen %d bits %04b\n",
		// target, node.data[:4], h, branchLen, bits&((2<<h)-1))

		lr := (bits >> h) & 1
		pos = (child(pos, p.height())) | lr
		// descend until we hit the bottom, populating as we go
		for {
			if node.niece[lr] == nil {
				node.niece[lr] = new(polNode)
				node.niece[lr].data = proofMap[pos]
				if node.niece[lr].data == empty {
					return fmt.Errorf("Wrote an empty hash h %d under %04x %d.niece[%d]\n",
						h, node.data[:4], pos, lr)
				}
				// fmt.Printf("h %d wrote %04x to %d\n", h, node.niece[lr].data[:4], pos)
				p.overWire++
			}

			if h == 0 {
				break
			}
			h--
			node = node.niece[lr]
			lr = (bits >> h) & 1
			pos = (child(pos, p.height()) ^ 2) | lr
		}

		// TODO do you need this at all?  If the Verify part already happend, maybe no
		// at bottom, populate target if needed
		// if we don't need this and take it out, will need to change the forget
		// pop above

		if node.niece[lr^1] == nil {
			node.niece[lr^1] = new(polNode)
			node.niece[lr^1].data = proofMap[pos^1]
			if node.niece[lr^1].data == empty {
				return fmt.Errorf("Wrote an empty hash h %d under %04x %d.niece[%d]\n",
					h, node.data[:4], pos, lr^1)
			}
			// p.overWire++ // doesn't count...? got it for free?
		}
	}
	return nil
}

// VerifyBlockProof takes a block proof and reconstructs / verifies it.
// takes a blockproof to verify, and the known correct tops to check against.
// also takes the number of leaves and forest height (those are redundant
// if we don't do weird stuff with overly-high forests, which we might)
// it returns a bool of whether the proof worked, and a map of the sparse
// forest in the blockproof
func (p *Pollard) VerifyBlockProof(
	bp BlockProof) (bool, map[uint64]Hash) {

	// if nothing to prove, it worked
	if len(bp.Targets) == 0 {
		return true, nil
	}

	height := p.height()
	tops := p.topHashesReverse()
	proofmap, err := bp.Reconstruct(p.numLeaves, height)
	if err != nil {
		fmt.Printf("VerifyBlockProof Reconstruct ERROR %s\n", err.Error())
		return false, proofmap
	}

	proofmap, err = p.matchKnownData(proofmap, bp)
	if err != nil {
		fmt.Printf("VerifyBlockProof matchKnownData ERROR %s\n", err.Error())
		return false, proofmap
	}

	//	fmt.Printf("Reconstruct complete\n")
	topposs, topheights := getTopsReverse(p.numLeaves, height)

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
			_, ok := proofmap[parpos]
			if !ok {
				// this will crash if either is 0000
				parhash := Parent(proofmap[left], proofmap[right])
				p.hashesEver++
				p.proofHashesEver++
				proofmap[parpos] = parhash
			}
			nextRow = append(nextRow, parpos)

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

// matchKnownData will run through all positions in the block
// proof, and follow them up. As soon as a hash is found that
// matches data in the pollard, we use the pollard from there
// onwards to fill a proofmap. Thus, this both checks if the
// proof amounts to something we already know (somewhere down
// the tree), and saves re-hashing things we already know.
func (p *Pollard) matchKnownData(
	rec map[uint64]Hash,
	bp BlockProof) (map[uint64]Hash, error) {
	proofmap := rec

	height := p.height()
	/*fmt.Printf("Targets: %v\n", bp.Targets)
	fmt.Printf("Num leaves: %d\n", p.numLeaves)
	for pos, hash := range rec {
		fmt.Printf("rec[%d] = %x\n", pos, hash[:4])
	}

	tops, _ := getTopsReverse(p.numLeaves, height)
	fmt.Printf("Tree top positions: %v, hashes: ", tops)
	tophashes := p.topHashesReverse()
	for _, th := range tophashes {
		fmt.Printf("%x ", th[:4])
	}
	fmt.Printf("\n")
	*/
	var left, right uint64

	for _, t := range bp.Targets {

		//fmt.Printf("Processing %d\n", t)
		// Fetch whatever is in the pollard on the path to
		// this leaf
		_, sibs, err := p.descendToPos(t)
		/*
			if err != nil {
				fmt.Printf("Error descendToPos: %s\n", err)
			}

			for i, s := range sibs {
				if s != nil {
					fmt.Printf("sibs[%d] = %x\n", i, s.data[:4])
				} else {
					fmt.Printf("sibs[%d] = nil\n", i)
				}
			}*/

		subH := detectSubTreeHeight(t, p.numLeaves, height)
		//fmt.Printf("Subtree height for [%d] is [%d]\n", left, subH)
		pos := up1(t, height)
		//fmt.Printf("We are at position [%d]\n", pos)
		right = t | 1
		left = right ^ 1
		leftHash, rightHash := rec[left], rec[right]

		found := false
		for h := uint8(1); h <= subH; h++ { // < or <=?  I think <
			_, ok := proofmap[pos]
			if !ok {
				if !found || sibs[h] == nil {
					//fmt.Printf("pos %d: Parent(%x, %x) -> ", pos, leftHash[:4], rightHash[:4])
					hash := Parent(leftHash, rightHash)
					//fmt.Printf("%x\n", hash[:4])
					p.hashesEver++
					p.proofHashesEver++
					if sibs[h] != nil {
						if sibs[h].data == hash {
							//fmt.Printf("Found matching hash in nodes[%d]\n", h)
							found = true
						}
					}
					proofmap[pos] = hash
				} else {
					//fmt.Printf("Using cached data after intersecting with pollard for %d\n", pos)
					proofmap[pos] = sibs[h].data
				}
			}
			if h == subH {
				break
			}

			if !found || sibs[h+1] == nil {
				hashes := map[uint64]Hash{}
				ok := false
				if pos&1 == 0 {
					//fmt.Printf("Left pos (calculated): [%d] - Right pos (from proof): [%d]\n", pos, pos^1)
					rightHash, ok = proofmap[pos^1]
					if !ok {
						rightHash, hashes, err = ResolveNode(proofmap, pos^1, height)
						for p, h := range hashes {
							proofmap[p] = h
						}
						if err != nil {
							return nil, err
						}
						p.hashesEver += uint64(len(hashes))
						p.proofHashesEver += uint64(len(hashes))
						p.proofResolveHashesEver += uint64(len(hashes))
					}
					leftHash = proofmap[pos]
				} else {
					//fmt.Printf("Left pos (from proof): [%d] - Right pos (calculated): [%d]\n", pos^1, pos)
					leftHash, ok = proofmap[pos^1]
					if !ok {
						leftHash, hashes, err = ResolveNode(proofmap, pos^1, height)
						for p, h := range hashes {
							proofmap[p] = h
						}
						if err != nil {
							return nil, err
						}
						p.hashesEver += uint64(len(hashes))
						p.proofHashesEver += uint64(len(hashes))
						p.proofResolveHashesEver += uint64(len(hashes))
					}
					rightHash = proofmap[pos]
				}
			}
			pos = up1(pos, height)
			//fmt.Printf("We are at [%d]\n", pos)
		}
	}

	return proofmap, nil
}
