/*
This package contains the tree structure used in the Utreexo accumulator library
It is meant to be coin agnostic. Bitcoin specific code can be found in 'cmd/'.

The basic flow for using the accumulator package goes as such:

1) Initialize Forest or Pollard.
	// inits a Forest in memory
	// pass a file in to init a forest on disk
	forest := accumulator.NewForest(nil)

	// declare pollard. No init function for pollard
	var pollard accumulator.Pollard

2) Call the Modify() methods for each one.
	// Adds and Deletes leaves from the Forest
	forest.Modify(leavesToAdd, leavesToDelete)

	// Adds and Deletes leaves from the Pollard
	pollard.Modify(leavesToAdd, leavesToDelete)

Thats it!

To add transaction verification, existence of the transaction needs to be
checked before to make sure the transaction exists. With Forest, this is done
with FindLeaf() which is a wrapper around Golang maps. This is ok since Forest
is aware of every tx in existence.

With Pollard, VerifyBatchProof() method needs to be called to check for tree
tops. If it verifies, this means that the transaction(s) that was sent over
exists.

Accumulator in detail:

Jargon:
In parts of the forest code you'll see these terminology being used.

	Perfect tree - A tree with 2**x leaves. All trees in Utreexo
	are perfect.
	Root - The top most parts of the tree. A Utreexo tree may have
	more than 1 top unlike a Merkle tree.
	Parent - The hash of two leaves concatenated.
	Sibling - The other leaf that shares the same parent.
	Aunt - The sibling of the parent leaf.
	Cousin - The children of the parent leaf's sibling.

Forest is a representation of a "full" Utreexo tree. The Forest implementation
would be the Utreexo bridge node. "Full" means that all the hashes of the tree
is stored.

This is done as either:

	1) byte slice
	2) contiguous data in a file

The ordering of the Utreexo tree is done in a similar fashion to that of a 2x2
array in row-major order. A Utreexo tree with 8 leaves would look like:

	06
	|-------\
	04......05
	|---\...|---\
	00..01..02..03

In the byte slice, this would be represented as such:

	byte[]{00, 01, 02, 03, 04, 05, 06}

It would be saved in the same fashion in the file.

For perfect trees, this is simple. However, for trees that aren't perfect, this
is a little different.

If one leaf gets added to the above tree, a new 8 leave tree is initialized by
Remap(). The new tree looks like such.

	em
	|---------------\
	12..............em
	|-------\.......|-------\
	08......09......em......em
	|---\...|---\...|---\...|---\
	00..01..02..03..04..em..em..em

em stands for empty and the leaf is initialized to either:

	1) uint64 of 0s for RAM
	2) whatever data there was on disk for on disk

The em leaves still have positions but just hold values of 0s.

Remap() is never called for when a leaf is deleted. For example, the forest
will hold an empty tree when leaf 04 is deleted in the above tree. It will
look like below:

	em
	|---------------\
	12..............em
	|-------\.......|-------\
	08......09......em......em
	|---\...|---\...|---\...|---\
	00..01..02..03..em..em..em..em

This is not a bug and is done on purpose for efficiency reasons. If a tree is
adding and deleting between a leaf number of 2**x, it will cause an io
bottleneck as the tree on the right is constantly deleted and re-initialized.

This will sometimes cause the Forest to have a total row value that is 1
greater than it actually is.

Pollard:

Pollard is a representation of a "sparse" Utreexo tree. "Sparse" means that
not all leaves are stored. The above tree with 8 leaves may look like such.
	14
	|---------------\
	**..............**
	|-------\.......|-------\
	**......**......**......**
	|---\...|---\...|---\...|---\
	**..**..**..**..**..**..**..**
Some leaves may be saved for caching purposes but they do not have to be.

Pollard uses binary tree pointers instead of a byte slice. Each hash is of type
PolNode. In a tree below, each number would represent a PolNode.
	14
	|---------------\
	12..............13
	|-------\.......|-------\
	08......09......10......11
	|---\...|---\...|---\...|---\
	00..01..02..03..04..05..06..07
Unlike traditional binary trees, the parent does NOT point to its children. It
points to its nieces.

Number 08 PolNode would point to leaves 02 and 03. 12 Polnode would point to
10 and 11. This is done for efficiency reasons as Utreexo Pollard uses the Aunt
to verify whether a leaf exists or not. Parents can be computed from its
children but an Aunt needs to be provided.

For example, with a tree like below, to prove the inclusion of 03, the prover
needs to provide 02, 03, 08, 13.
	14
	|---------------\
	12..............13
	|-------\.......|-------\
	08......09......10......11
	|---\...|---\...|---\...|---\
	00..01..02..03..04..05..06..07

A Pollard node is not aware of each individual PolNode's position in the tree.
This is different from Forest, as Forest is aware of every single leaf and its
position. Getting the position of the Polnode is done through grabPos()/descendToPos()
methods.

*/
package accumulator
