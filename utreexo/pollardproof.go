package utreexo

import (
	"fmt"
)

// IngestBlockProof populates the Pollard with all needed data to delete the
// targets in the block proof
func (p *Pollard) IngestBlockProof(bp BlockProof) error {
	var empty Hash
	// TODO so many things to change
	ok, proofMap := VerifyBlockProof(
		bp, p.topHashesReverse(), p.numLeaves, p.height())
	if !ok {
		return fmt.Errorf("block proof mismatch")
	}
	//	fmt.Printf("targets: %v\n", bp.Targets)
	// go through each target and populate pollard
	for _, target := range bp.Targets {

		tNum, branchLen, bits := detectOffset(target, p.numLeaves)
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
					return fmt.Errorf("Wrote an empty hash h %d under %04x %d.niece[%d]",
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
				return fmt.Errorf("Wrote an empty hash h %d under %04x %d.niece[%d]",
					h, node.data[:4], pos, lr^1)
			}
			// p.overWire++ // doesn't count...? got it for free?
		}
	}
	return nil
}
