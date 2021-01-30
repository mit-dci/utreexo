package accumulator

import (
	"encoding/binary"
	"fmt"
	"io"
)

// BatchProof :
type BatchProof struct {
	Targets []uint64
	Proof   []Hash
	// list of leaf locations to delete, along with a bunch of hashes that
	// give the proof.
	// the position of the hashes is implied / computable from the leaf positions
}

type miniTree struct {
	l, r, parent node // left, right, parent
}

/*
Batchproof serialization is:
4bytes numTargets
4bytes numHashes
[]Targets (8 bytes each)
[]Hashes (32 bytes each)
*/

// Serialize a batchproof to a writer.
func (bp *BatchProof) Serialize(w io.Writer) (err error) {
	// first write the number of targets (4 byte uint32)
	err = binary.Write(w, binary.BigEndian, uint32(len(bp.Targets)))
	if err != nil {
		return err
	}
	// if there are no targets, finish early & don't write proofs
	// if len(bp.Targets) == 0 {
	// return
	// }
	// write out number of hashes in the proof
	err = binary.Write(w, binary.BigEndian, uint32(len(bp.Proof)))
	if err != nil {
		return
	}

	// write out each target
	for _, t := range bp.Targets {
		// there's no need for these to be 64 bit for the next few decades...
		err = binary.Write(w, binary.BigEndian, t)
		if err != nil {
			return
		}
	}

	// then the rest is just hashes
	for _, h := range bp.Proof {
		_, err = w.Write(h[:])
		if err != nil {
			return
		}
	}
	return
}

// TODO: could make this more efficient by not encoding as much empty stuff

func (bp *BatchProof) SerializeSize() int {
	// empty batchProofs are 4 bytes
	// if len(bp.Targets) == 0 {
	// 	return 4
	// }
	// 8B for numTargets and numHashes, 8B per target, 32B per hash
	return 8 + (8 * (len(bp.Targets))) + (32 * (len(bp.Proof)))
}

// Deserialize gives a block proof back from the serialized bytes
func (bp *BatchProof) Deserialize(r io.Reader) (err error) {
	var numTargets, numHashes uint32
	err = binary.Read(r, binary.BigEndian, &numTargets)
	if err != nil {
		return
	}

	if numTargets > 1<<16 {
		err = fmt.Errorf("%d targets - too many\n", numTargets)
		return
	}

	// read number of hashes
	err = binary.Read(r, binary.BigEndian, &numHashes)
	if err != nil {
		fmt.Printf("bp deser err %s\n", err.Error())
		return
	}
	if numHashes > 1<<16 {
		err = fmt.Errorf("%d hashes - too many\n", numHashes)
		return
	}

	bp.Targets = make([]uint64, numTargets)
	for i, _ := range bp.Targets {
		err = binary.Read(r, binary.BigEndian, &bp.Targets[i])
		if err != nil {
			fmt.Printf("bp deser err %s\n", err.Error())
			return
		}
	}

	bp.Proof = make([]Hash, numHashes)
	for i, _ := range bp.Proof {
		_, err = io.ReadFull(r, bp.Proof[i][:])
		if err != nil {
			fmt.Printf("bp deser err %s\n", err.Error())
			if err == io.EOF && i == len(bp.Proof) {
				err = nil // EOF at the end is not an error...
			}
			return
		}
	}
	return
}

// ToString for debugging, shows the blockproof
func (bp *BatchProof) ToString() string {
	s := fmt.Sprintf("%d targets: ", len(bp.Targets))
	for _, t := range bp.Targets {
		s += fmt.Sprintf("%d ", t)
	}
	s += fmt.Sprintf("\n%d proofs: ", len(bp.Proof))
	for _, p := range bp.Proof {
		s += fmt.Sprintf("%04x\t", p[:4])
	}
	s += "\n"
	return s
}

// TODO :
/*
several changes needed & maybe easier to do them incrementally but at this
point it's more of a rewrite.
The batchProof no longer contains target hashes; those are obtained separately
from the leaf data.  This makes sense as the verifying node will have to
know the preimages anyway to do tx/sig checks, so they can also compute the
hashes themselves instead of receiving them.

prior to this change: verifyBatchProof() verifies up to the roots,
and then returned all the new stuff it received / computed, so that it
could be populated into the pollard (to allow for subsequent deletion)

the new way it works: verifyBatchProof() and IngestBatchProof() will be
merged, since really right now IngestBatchProof() is basically just a wrapper
for verifyBatchProof().  It will get a batchProof as well as a slice of
target hashes (the things being proven).  It will hash up to known branches,
then not return anything as it's populating as it goes.  If the ingestion fails,
we need to undo everything added.  It's also ok to trim everything down to
just the roots in that case for now; can add the backtrack later
(it doesn't seem too hard if you just keep track of every new populated position,
then wipe them on an invalid proof.  Though... if you want to be really
efficient / DDoS resistant, only wipe the invalid parts and leave the partially
checked stuff that works.


*/

// verifyBatchProof verifies a batchproof by checking against the set of known
// correct roots.
// Takes a BatchProof, the accumulator roots, and the number of leaves in the forest.
// Returns wether or not the proof verified correctly, the partial proof tree,
// and the subset of roots that was computed.
func (p *Pollard) verifyBatchProof(
	bp BatchProof, targs []Hash) ([]miniTree, []node, error) {
	if len(bp.Targets) == 0 {
		return nil, nil, nil
	}
	fmt.Printf("got proof %s\n", bp.ToString())

	rootHashes := p.rootHashesReverse()
	// copy targets to leave them in original order
	targets := make([]uint64, len(bp.Targets))
	copy(targets, bp.Targets)
	sortUint64s(targets)

	rows := treeRows(p.numLeaves)
	proofPositions := ProofPositions(targets, p.numLeaves, rows)
	numComputable := len(targets)
	// The proof should have as many hashes as there are proof positions.
	if len(proofPositions) != len(bp.Proof) {
		// fmt.Printf(")
		return nil, nil,
			fmt.Errorf("verifyBatchProof %d proofPositions but %d proof hashes",
				len(proofPositions), len(bp.Proof))
	}

	// targetNodes holds nodes that are known, on the bottom row those
	// are the targets, on the upper rows it holds computed nodes.
	// rootCandidates holds the roots that where computed, and have to be
	// compared to the actual roots at the end.
	targetNodes := make([]node, 0, len(targets)*int(rows))
	rootCandidates := make([]node, 0, len(rootHashes))
	// trees holds the entire proof tree of the batchproof, sorted by parents.
	trees := make([]miniTree, 0, numComputable)
	// initialise the targetNodes for row 0.
	// TODO: this would be more straight forward if bp.Proofs wouldn't
	// contain the targets
	// TODO targets are now given in a separate argument
	// bp.Proofs is now on from ProofPositions()
	proofHashes := make([]Hash, 0, len(proofPositions))
	var targetsMatched uint64
	for len(targets) > 0 {

		// a row-0 root should never be given, as it can only be a target and
		// targets aren't sent

		// `targets` might contain a target and its sibling or just the target, if
		// only the target is present the sibling will be in `proofPositions`.

		if uint64(len(proofPositions)) > targetsMatched &&
			targets[0]^1 == proofPositions[targetsMatched] {
			// target's sibling is in proof positions.
			lr := targets[0] & 1
			targetNodes = append(targetNodes, node{Pos: targets[0], Val: bp.Proof[lr]})
			proofHashes = append(proofHashes, bp.Proof[lr^1])
			targetsMatched++
			bp.Proof = bp.Proof[2:]
			targets = targets[1:]
			continue
		}

		// the sibling is not included in proof positions, therefore
		// it must also be a target. if there are fewer than 2 proof
		// hashes or less than 2 targets left the proof is invalid because
		// there is a target without matching proof.
		if len(bp.Proof) < 2 || len(targets) < 2 {
			return nil, nil, fmt.Errorf("verifyBatchProof ran out of proof hashes")
		}

		// if we got this far there are 2 targets that are siblings; pop em both
		targetNodes = append(targetNodes,
			node{Pos: targets[0], Val: bp.Proof[0]},
			node{Pos: targets[1], Val: bp.Proof[1]})
		bp.Proof = bp.Proof[2:]
		targets = targets[2:]
	}

	proofHashes = append(proofHashes, bp.Proof...)
	bp.Proof = proofHashes

	// hash every target node with its sibling (which either is contained
	// in the proof or also a target)
	for len(targetNodes) > 0 {
		var target, proof node
		target = targetNodes[0]
		if len(proofPositions) > 0 && target.Pos^1 == proofPositions[0] {
			// target has a sibling in the proof positions, fetch proof
			proof = node{Pos: proofPositions[0], Val: bp.Proof[0]}
			proofPositions = proofPositions[1:]
			bp.Proof = bp.Proof[1:]
			targetNodes = targetNodes[1:]
		} else {
			// target should have its sibling in targetNodes
			if len(targetNodes) == 1 {
				// sibling not found
				return nil, nil, fmt.Errorf("%v sibling not found", targetNodes)
			}

			proof = targetNodes[1]
			targetNodes = targetNodes[2:]
		}

		// figure out which node is left and which is right
		left := target
		right := proof
		if target.Pos&1 == 1 {
			right, left = left, right
		}

		// get the hash of the parent from the cache or compute it
		parentPos := parent(target.Pos, rows)
		hash := parentHash(left.Val, right.Val)

		populatedNode, _, _, err := p.grabPos(parentPos)
		if err != nil {
			return nil, nil, fmt.Errorf("verify grabPos error %s", err.Error())
		}
		if populatedNode != nil && populatedNode.data != empty &&
			hash != populatedNode.data {
			// The hash did not match the cached hash
			return nil, nil, fmt.Errorf("verifyBatchProof pos %d have %x calc'd %x",
				parentPos, populatedNode.data, hash)
		}

		trees = append(trees,
			miniTree{parent: node{Val: hash, Pos: parentPos}, l: left, r: right})

		row := detectRow(parentPos, rows)
		if p.numLeaves&(1<<row) > 0 && parentPos ==
			rootPosition(p.numLeaves, row, rows) {
			// the parent is a root -> store as candidate, to check against
			// actual roots later.
			rootCandidates = append(rootCandidates, node{Val: hash, Pos: parentPos})
			continue
		}
		targetNodes = append(targetNodes, node{Val: hash, Pos: parentPos})
	}

	if len(rootCandidates) == 0 {
		// no roots to verify
		return nil, nil, fmt.Errorf("verifyBatchProof no roots")
	}

	// `roots` is ordered, therefore to verify that `rootCandidates`
	// holds a subset of the roots
	// we count the roots that match in order.
	rootMatches := 0
	for _, root := range rootHashes {
		if len(rootCandidates) > rootMatches &&
			root == rootCandidates[rootMatches].Val {
			rootMatches++
		}
	}
	if len(rootCandidates) != rootMatches {
		// the proof is invalid because some root candidates were not
		// included in `roots`.
		return nil, nil, fmt.Errorf("verifyBatchProof missing roots")
	}

	return trees, rootCandidates, nil
}
