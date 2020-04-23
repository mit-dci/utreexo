/*
This package contains the tree structure used in the Utreexo accumulator library
It is meant to be coin agnostic. Bitcoin specific code can be found in 'cmd/'.

Jargon:

In parts of the code you'll see these terminology being used.

	Perfect tree - A tree with 2**x leaves.
	Sibling -

Forest:

Forest is a representation of a "full" Utreexo tree. The Forest implementation
would be the Utreexo bridge node. "Full" means that all the hashes of the tree
is stored.

This is done as either:

	1) byte slice
	2) contingous data in a file

The ordering of the Utreexo tree is done in a similar fashion to that of a 2x2
array in row-major order. A Utreexo tree with 8 leaves would look like:

	06
	|-------\
	04      05
	|---\   |---\
	00  01  02  03

In the byte slice, this would be represented as such:

	byte[]{00, 01, 02, 03, 04, 05, 06}

It would be saved in the same fashion in the file.

For perfect trees, this is simple. However, for trees that aren't perfect, this
is a little different.

If one leaf gets added to the above tree, a new 8 leave tree is initialized by
Remap(). The new tree looks like such.

	em
	|---------------\
	12              em
	|-------\       |-------\
	08      09      em      em
	|---\   |---\   |---\   |---\
	00  01  02  03  04  em  em  em

em stands for empty and the leaf is initialized to either:

	1) uint64 of 0s for RAM
	2) whatever data there was on disk for on disk

The em leaves still have positions but just hold values of 0s.

Remap() is never called for when a leaf is deleted. For example, the forest
will hold an empty tree when leaf 04 is deleted in the above tree. It will
look like below:

	em
	|---------------\
	12              em
	|-------\       |-------\
	08      09      em      em
	|---\   |---\   |---\   |---\
	00  01  02  03  em  em  em  em

This is not a bug and is done on purpose for efficiency reasons. If a tree is
adding and deleting between a leaf number of 2**x, it will cause an io
bottleneck as the tree on the right is constantly deleted and re-initialized.

Pollard:

Pollard is a representation of a "sparse" Utreexo tree. "Sparse" means that
not all leaves are stored. The above tree with 8 leaves may look like such.

	14
	|---------------\

	|-------\       |-------\

	|---\   |---\   |---\   |---\

Some leaves may be saved for caching purposes but they do not have to be.

Pollard uses binary tree pointers instead of a byte slice.
*/
package accumulator
