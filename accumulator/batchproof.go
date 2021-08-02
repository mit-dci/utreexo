package accumulator

import (
	"bytes"
	"fmt"
	"io"

	"github.com/btcsuite/btcd/wire"
)

// BatchProof is the inclusion-proof for multiple leaves.
type BatchProof struct {
	// Targets are the ist of leaf locations to delete. These are the bottommost leaves.
	// With the tree below, the Targets can only consist of one of these: 00, 01, 02, 03
	//
	// 06
	// |-------\
	// 04      05
	// |---\   |---\
	// 00  01  02  03
	Targets []uint64

	// All the nodes in the tree that are needed to hash up to the root of
	// the tree. Here, the root is 06. If Targets are [00, 01], then Proof
	// would be [05] as you need 04 and 05 to hash to 06. 04 can be calculated
	// by hashing 00 and 01.
	//
	// 06
	// |-------\
	// 04      05
	// |---\   |---\
	// 00  01  02  03
	Proof []Hash
}

// Serialize serializes a batchproof to a writer.
func (bp *BatchProof) Serialize(w io.Writer) (err error) {
	// Batchproof serialization is, in order:
	// 4bytes numTargets
	// 4bytes numHashes
	// []Targets (8 bytes each)
	// []Hashes (32 bytes each)

	// first write the number of targets (4 byte uint32)
	err = wire.WriteVarInt(w, 0, uint64(len(bp.Targets)))
	if err != nil {
		return err
	}

	// write out number of hashes in the proof
	err = wire.WriteVarInt(w, 0, uint64(len(bp.Proof)))
	if err != nil {
		return
	}

	// write out each target
	for _, t := range bp.Targets {
		err = wire.WriteVarInt(w, 0, t)
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
	err := wire.WriteVarInt(buf, 0, uint64(len(bp.Targets)))
	if err != nil {
		return nil, err
	}

	// write out number of hashes in the proof
	err = wire.WriteVarInt(buf, 0, uint64(len(bp.Proof)))
	if err != nil {
		return nil, err
	}

	// write out each target
	for _, t := range bp.Targets {
		err = wire.WriteVarInt(buf, 0, t)
		if err != nil {
			return nil, err
		}
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

// SerializeSize returns the number of bytes it would take to serialize
// the BatchProof.
func (bp *BatchProof) SerializeSize() int {
	var size int
	size += wire.VarIntSerializeSize(uint64(len(bp.Targets)))
	for _, t := range bp.Targets {
		size += wire.VarIntSerializeSize(t)
	}

	size += wire.VarIntSerializeSize(uint64(len(bp.Proof)))
	size += len(bp.Proof) * 32

	return size
}

// Deserialize gives a BatchProof back from a reader.
func (bp *BatchProof) Deserialize(r io.Reader) (err error) {
	numTargets, err := wire.ReadVarInt(r, 0)
	if err != nil {
		return
	}

	if numTargets > 1<<16 {
		err = fmt.Errorf("%d targets - too many\n", numTargets)
		return
	}

	// read number of hashes
	numHashes, err := wire.ReadVarInt(r, 0)
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
		bp.Targets[i], err = wire.ReadVarInt(r, 0)
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
	reader := bytes.NewReader(serialized)

	numTargets, err := wire.ReadVarInt(reader, 0)
	if err != nil {
		return nil, err
	}

	if numTargets > 1<<16 {
		err = fmt.Errorf("%d targets - too many\n", numTargets)
		return nil, err
	}

	// read number of hashes
	numHashes, err := wire.ReadVarInt(reader, 0)
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
		bp.Targets[i], err = wire.ReadVarInt(reader, 0)
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

// miniTree is a tree of height 1 that holds a parent and its children along with
// metadata.
type miniTree struct {
	tree       uint8
	branchLen  uint8
	parent     node
	leftChild  node
	rightChild node
}

// targPos is just targets with their hashes. Used for sorting
type targPos struct {
	pos uint64
	val Hash
}

// verifyBatchProof verifies a batchproof by checking against the set of known
// correct roots.
// Takes a BatchProof, the accumulator roots, and the number of leaves in the forest.
// Returns wether or not the proof verified correctly, the partial proof tree,
// and the subset of roots that was computed.
//
// NOTE: targetHashes MUST be in the same order they were proven in. (aka they
// have to be in the same order as hashes given to ProveBatch(). In Bitcoin's
// case, this would be the order in which they appear in a block.
//
// TODO OH WAIT -- this is not how to to it!  Don't hash all the way up to the
// roots to verify -- just hash up to any populated node!  Saves a ton of CPU!
func verifyBatchProof(targetHashes []Hash, bp BatchProof, roots []Hash, numLeaves uint64,
	// cached should be a function that fetches nodes from the pollard and
	// indicates whether they exist or not, this is only useful for the pollard
	// and nil should be passed for the forest.
	cached func(pos uint64) (bool, Hash)) (bool, [][]miniTree, []node) {

	// If there is nothing to prove, return true
	if len(bp.Targets) == 0 {
		return true, nil, nil
	}
	// There should be a hash for each of the targets being proven
	if len(bp.Targets) != len(targetHashes) {
		return false, nil, nil
	}

	tPos := make([]targPos, len(bp.Targets))

	for i, hash := range targetHashes {
		tPos[i].val = hash
		tPos[i].pos = bp.Targets[i]
	}

	sortTargPos(tPos)

	sortedDelHashes := make([]Hash, len(bp.Targets))
	targets := make([]uint64, len(bp.Targets))
	for i, t := range tPos {
		sortedDelHashes[i] = t.val
		targets[i] = t.pos
	}

	targetHashes = sortedDelHashes

	if cached == nil {
		cached = func(_ uint64) (bool, Hash) { return false, empty }
	}

	rows := treeRows(numLeaves)
	proofPositions := NewPositionList()
	defer proofPositions.Free()

	// Grab all the positions needed to prove the targets
	ProofPositions(targets, numLeaves, rows, &proofPositions.list)

	// The proof should have as many hashes as there are proof positions.
	if len(proofPositions.list) != len(bp.Proof) {
		return false, nil, nil
	}

	// targetNodes holds nodes that are known, on the bottom row those
	// are the targets, on the upper rows it holds computed nodes.
	// rootCandidates holds the roots that where computed, and have to be
	// compared to the actual roots at the end.
	targetNodes := make([]node, 0, len(targets)*int(rows))
	rootCandidates := make([]node, 0, len(roots))

	// trees holds the entire proof tree of the batchproof. MiniTrees are
	// grouped by which root they are a part of. These miniTrees are then
	// also sorted by the parent's position in ascending order.
	trees := make([][]miniTree, len(roots))

	// initialise the targetNodes for row 0.
	proofHashes := make([]Hash, 0, len(proofPositions.list))
	var targetsMatched uint64
	for len(targets) > 0 {
		// check if the target is the row 0 root.
		// this is the case if its the last leaf (pos==numLeaves-1)
		// AND the tree has a root at row 0 (numLeaves&1==1)
		if targets[0] == numLeaves-1 && numLeaves&1 == 1 {
			// target is the row 0 root, append it to the root candidates.
			rootCandidates = append(rootCandidates,
				node{Val: roots[len(roots)-1], Pos: targets[0]})
			break
		}

		// `targets` might contain a target and its sibling or just the target, if
		// only the target is present the sibling will be in `proofPositions`.
		if uint64(len(proofPositions.list)) > targetsMatched &&
			targets[0]^1 == proofPositions.list[targetsMatched] {
			targetNodes = append(targetNodes, node{Pos: targets[0], Val: targetHashes[0]})
			proofHashes = append(proofHashes, bp.Proof[0])

			targetsMatched++
			bp.Proof = bp.Proof[1:]
			targets = targets[1:]
			targetHashes = targetHashes[1:]
			continue
		}

		// the sibling is not included in the proof positions, therefore
		// it has to be included in targets. if there are less than 2 proof
		// hashes or less than 2 targets left the proof is invalid because
		// there is a target without matching proof.
		if len(targetHashes) < 2 || len(targets) < 2 {
			return false, nil, nil
		}

		targetNodes = append(targetNodes,
			node{Pos: targets[0], Val: targetHashes[0]},
			node{Pos: targets[1], Val: targetHashes[1]})

		targetHashes = targetHashes[2:]
		targets = targets[2:]
	}

	proofHashes = append(proofHashes, bp.Proof...)
	bp.Proof = proofHashes

	// hash every target node with its sibling (which either is contained
	// in the proof or also a target)
	for len(targetNodes) > 0 {
		var target, proof node
		target = targetNodes[0]

		if len(proofPositions.list) > 0 && target.Pos^1 == proofPositions.list[0] {
			// target has a sibling in the proof positions, fetch proof
			proof = node{Pos: proofPositions.list[0], Val: bp.Proof[0]}
			proofPositions.list = proofPositions.list[1:]
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

		// check if the parent is cached
		parentPos := parent(target.Pos, rows)
		isParentCached, cachedParent := cached(parentPos)

		var hash Hash
		// if parent is cached, also check if the left and right is cached.
		// if they're all there, no need to re-hash for the parent.
		if isParentCached {
			isLeftCached, cachedLeft := cached(left.Pos)
			isRightCached, cachedRight := cached(right.Pos)

			if isRightCached && isLeftCached {
				if left.Val == cachedLeft &&
					right.Val == cachedRight {
					hash = cachedParent
				} else {
					// The left and right did not match the cached
					// left and right.
					return false, nil, nil
				}
			} else {
				hash = parentHash(left.Val, right.Val)
				if hash != cachedParent {
					// The calculated hash did not match the cached parent.
					return false, nil, nil
				}
			}
		} else {
			hash = parentHash(left.Val, right.Val)
		}

		// sort the miniTrees by which tree they are in
		tree, branchLen, _ := detectOffset(parentPos, numLeaves)
		trees[tree] = append(trees[tree], miniTree{
			tree:       tree,
			branchLen:  branchLen,
			parent:     node{Val: hash, Pos: parentPos},
			leftChild:  left,
			rightChild: right,
		})

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

	if len(positionList.list) != len(bp.Proof) {
		return nil, fmt.Errorf("Reconstruct wants %d hashes, has %d",
			len(positionList.list), len(bp.Proof))
	}

	for i, pos := range positionList.list {
		proofTree[pos] = bp.Proof[i]
	}

	return proofTree, nil
}
