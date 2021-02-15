package accumulator

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// BatchProof :
type BatchProof struct {
	Targets []uint64
	Proof   []Hash
	// list of leaf locations to delete, along with a bunch of hashes that give the proof.
	// the position of the hashes is implied / computable from the leaf positions
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
	// write out number of hashes in the proof
	err = binary.Write(w, binary.BigEndian, uint32(len(bp.Proof)))
	if err != nil {
		return
	}

	// write out each target
	for _, t := range bp.Targets {
		// there's no need for these to be 64 bit for the next few decades...
		err = binary.Write(w, binary.BigEndian, uint64(t))
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

// SerializeBytes serializes and returns the batchproof as raw bytes
// the serialization is the same as Serialize() method
func (bp *BatchProof) SerializeBytes() ([]byte, error) {
	size := bp.SerializeSize()

	b := make([]byte, size)
	buf := bytes.NewBuffer(b)

	// first write the number of targets (4 byte uint32)
	err := binary.Write(buf, binary.BigEndian, uint32(len(bp.Targets)))
	if err != nil {
		return nil, err
	}

	// write out number of hashes in the proof
	err = binary.Write(buf, binary.BigEndian, uint32(len(bp.Proof)))
	if err != nil {
		return nil, err
	}

	// then the rest is just hashes
	for _, h := range bp.Proof {
		_, err = buf.Write(h[:])
		if err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
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

// DeserializeBPFromBytes, given serialized bytes, returns a pointer to the
// deserialized batchproof. The deserialization is the same as Deserialize() method
// on BatchProof
func DeserializeBPFromBytes(serialized []byte) (*BatchProof, error) {
	var numTargets, numHashes uint32

	reader := bytes.NewReader(serialized)

	err := binary.Read(reader, binary.BigEndian, &numTargets)
	if err != nil {
		return nil, err
	}

	if numTargets > 1<<16 {
		err = fmt.Errorf("%d targets - too many\n", numTargets)
		return nil, err
	}

	// read number of hashes
	err = binary.Read(reader, binary.BigEndian, &numHashes)
	if err != nil {
		str := fmt.Errorf("bp deser err %s\n", err.Error())
		return nil, str
	}

	if numHashes > 1<<16 {
		err = fmt.Errorf("%d hashes - too many\n", numHashes)
		return nil, err
	}

	bp := BatchProof{}

	bp.Targets = make([]uint64, numTargets)
	for i, _ := range bp.Targets {
		err = binary.Read(reader, binary.BigEndian, &bp.Targets[i])
		if err != nil {
			str := fmt.Errorf("bp deser err %s\n", err.Error())
			return nil, str
		}
	}

	bp.Proof = make([]Hash, numHashes)

	for i, _ := range bp.Proof {
		_, err = io.ReadFull(reader, bp.Proof[i][:])
		if err != nil {
			if err == io.EOF && i == len(bp.Proof) {
				err = nil // EOF at the end is not an error...
			} else {
				str := fmt.Errorf("bp deser err %s\n", err.Error())
				return nil, str
			}
		}
	}

	return &bp, nil
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

// TODO OH WAIT -- this is not how to to it!  Don't hash all the way up to the
// roots to verify -- just hash up to any populated node!  Saves a ton of CPU!

// verifyBatchProof verifies a batchproof by checking against the set of known
// correct roots.
// Takes a BatchProof, the accumulator roots, and the number of leaves in the forest.
// Returns wether or not the proof verified correctly, the partial proof tree,
// and the subset of roots that was computed.
func verifyBatchProof(bp BatchProof, roots []Hash, numLeaves uint64,
	// cached should be a function that fetches nodes from the pollard and
	// indicates whether they exist or not, this is only useful for the pollard
	// and nil should be passed for the forest.
	cached func(pos uint64) (bool, Hash)) (bool, [][3]node, []node) {
	if len(bp.Targets) == 0 {
		return true, nil, nil
	}

	// copy targets to leave them in original order
	targets := make([]uint64, len(bp.Targets))
	copy(targets, bp.Targets)
	sortUint64s(targets)

	if cached == nil {
		cached = func(_ uint64) (bool, Hash) { return false, empty }
	}

	rows := treeRows(numLeaves)
	positionList := NewPositionList()
	defer positionList.Free()

	computablePositions :=
		ProofPositions(targets, numLeaves, rows, &positionList.list)

	// The proof should have as many hashes as there are proof positions.
	if len(positionList.list)+len(bp.Targets) != len(bp.Proof) {
		return false, nil, nil
	}

	// targetNodes holds nodes that are known, on the bottom row those
	// are the targets, on the upper rows it holds computed nodes.
	// rootCandidates holds the roots that where computed, and have to be
	// compared to the actual roots at the end.
	targetNodes := make([]node, 0, len(targets)*int(rows))
	rootCandidates := make([]node, 0, len(roots))
	// trees is a slice of 3-Tuples, each tuple represents a parent and its children.
	// tuple[0] is the parent, tuple[1] is the left child and tuple[2]
	// is the right child.
	// trees holds the entire proof tree of the batchproof in this way,
	// sorted by the tuple[0].
	trees := make([][3]node, 0, computablePositions)
	// initialise the targetNodes for row 0.
	// TODO: this would be more straight forward if bp.Proofs wouldn't
	// contain the targets
	proofHashes := make([]Hash, 0, len(positionList.list))
	var targetsMatched uint64
	for len(targets) > 0 {
		// check if the target is the row 0 root.
		// this is the case if its the last leaf (pos==numLeaves-1)
		// AND the tree has a root at row 0 (numLeaves&1==1)
		if targets[0] == numLeaves-1 && numLeaves&1 == 1 {
			// target is the row 0 root, append it to the root candidates.
			rootCandidates = append(rootCandidates,
				node{Val: roots[len(roots)-1], Pos: targets[0]})
			bp.Proof = bp.Proof[1:]
			break
		}

		// `targets` might contain a target and its sibling or just the target, if
		// only the target is present the sibling will be in `proofPositions`.
		if uint64(len(positionList.list)) > targetsMatched &&
			targets[0]^1 == positionList.list[targetsMatched] {
			// the sibling of the target is included in the proof positions.
			lr := targets[0] & 1
			targetNodes = append(targetNodes, node{Pos: targets[0], Val: bp.Proof[lr]})
			proofHashes = append(proofHashes, bp.Proof[lr^1])
			targetsMatched++
			bp.Proof = bp.Proof[2:]
			targets = targets[1:]
			continue
		}

		// the sibling is not included in the proof positions, therefore
		// it has to be included in targets. if there are less than 2 proof
		// hashes or less than 2 targets left the proof is invalid because
		// there is a target without matching proof.
		if len(bp.Proof) < 2 || len(targets) < 2 {
			return false, nil, nil
		}

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
		if len(positionList.list) > 0 && target.Pos^1 == positionList.list[0] {
			// target has a sibling in the proof positions, fetch proof
			proof = node{Pos: positionList.list[0], Val: bp.Proof[0]}
			positionList.list = positionList.list[1:]
			bp.Proof = bp.Proof[1:]
			targetNodes = targetNodes[1:]
		} else {
			// target should have its sibling in targetNodes
			if len(targetNodes) == 1 {
				// sibling not found
				return false, nil, nil
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
		isParentCached, cachedHash := cached(parentPos)
		hash := parentHash(left.Val, right.Val)
		if isParentCached && hash != cachedHash {
			// The hash did not match the cached hash
			return false, nil, nil
		}

		trees = append(trees, [3]node{{Val: hash, Pos: parentPos}, left, right})

		row := detectRow(parentPos, rows)
		if numLeaves&(1<<row) > 0 && parentPos == rootPosition(numLeaves, row, rows) {
			// the parent is a root -> store as candidate, to check against
			// actual roots later.
			rootCandidates = append(rootCandidates, node{Val: hash, Pos: parentPos})
			continue
		}
		targetNodes = append(targetNodes, node{Val: hash, Pos: parentPos})
	}

	if len(rootCandidates) == 0 {
		// no roots to verify
		return false, nil, nil
	}

	// `roots` is ordered, therefore to verify that `rootCandidates`
	// holds a subset of the roots
	// we count the roots that match in order.
	rootMatches := 0
	for i, _ := range roots {
		if len(rootCandidates) > rootMatches &&
			roots[len(roots)-(i+1)] == rootCandidates[rootMatches].Val {
			rootMatches++
		}
	}
	if len(rootCandidates) != rootMatches {
		// the proof is invalid because some root candidates were not
		// included in `roots`.
		return false, nil, nil
	}

	return true, trees, rootCandidates
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

	// copy bp.targets and send copy
	targets := make([]uint64, len(bp.Targets))
	copy(targets, bp.Targets)
	sortUint64s(targets)

	positionList := NewPositionList()
	defer positionList.Free()

	ProofPositions(targets, numleaves, forestRows, &positionList.list)
	positionList.list = mergeSortedSlices(targets, positionList.list)

	if len(positionList.list) != len(bp.Proof) {
		return nil, fmt.Errorf("Reconstruct wants %d hashes, has %d",
			len(positionList.list), len(bp.Proof))
	}

	for i, pos := range positionList.list {
		proofTree[pos] = bp.Proof[i]
	}

	return proofTree, nil
}
