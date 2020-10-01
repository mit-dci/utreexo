CowForest
===================

Utreexo on disk is formatted as a redirect-on-write structure. I like to call it CowForest.

*because copy-on-write forest "cowForest" sounds cooler than "RowForest" and less confusing because
the term "row" is used often elsewhere.*

## Structure

A CowForest consists of the following:

1. TreeBlocks
2. TreeTables
3. Manifest
3. Metadata

TreeBlocks are the smallest Utreexo merkle tree structure of a CowForest.

TreeTables are multitude of TreeBlocks that are grouped.

Manifest holds all the metadata needed for reading a CowForest.

Metadata is all the CowForest data that is needed but isn't saved to disk

### TreeBlock

```
   <beginning of file>
   [treeBlock #1]
   [treeBlock #2]
   ...
   ...
   ...
   [treeBlock #n]
   <end of file>
```

Each treeBlock is a fixed size utreexo tree with height n that holds a total of
2**(n+1) - 1 nodes. This tree can hold 2**n bottom leaves.
A treeBlock may represent any height of a utreexo forest. For example this forestRows = 4 Forest:

```
   30
   |-------------------------------\
   28                              29
   |---------------\               |---------------\
   24              25              26              27
   |-------\       |-------\       |-------\       |-------\
   16      17      18      19      20      21      22      23
   |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\
   00  01  02  03  04  05  06  07  08  09  10  11  12  13  14  15
```

would be organized into treeBlocks are organized into trees with forestRows = 3,
then this forest would have 3 treeBlocks which are:

```
   28
   |---------------\
   24              25
   |-------\       |-------\
   16      17      18      19
   |---\   |---\   |---\   |---\
   00  01  02  03  04  05  06  07
```

```
   29
   |---------------\
   26              27
   |-------\       |-------\
   20      21      22      23
   |---\   |---\   |---\   |---\
   08  09  10  11  12  13  14  15
```

```
   em
   |---------------\
   em              em
   |-------\       |-------\
   em      em      em      em
   |---\   |---\   |---\   |---\
   30  em  em  em  em  em  em  em
```

TreeBlocks aren't aware of their position within the enitre Utreexo forest. For the above
three TreeBlocks, they would actually have their nodes numbered as such:

```
   28
   |---------------\
   24              25
   |-------\       |-------\
   16      17      18      19
   |---\   |---\   |---\   |---\
   00  01  02  03  04  05  06  07
```

```
   28
   |---------------\
   24              25
   |-------\       |-------\
   16      17      18      19
   |---\   |---\   |---\   |---\
   00  01  02  03  04  05  06  07
```

```
   em
   |---------------\
   em              em
   |-------\       |-------\
   em      em      em      em
   |---\   |---\   |---\   |---\
   00  em  em  em  em  em  em  em
```

To access the correct node in relation to the entire forest, the positions must be translated
into the local positions for each individual treeBlocks. This is done with function gPosToLocPos()

TreeTables themselves have a "row", which is their row position within the entire Utreexo forest.
In the above example of this Utreexo forest with each TreeBlock having forestRows = 3,
there would need to be 2 TreeBlockRows.

```
  TreeBlockRow 1 30
                 |-------------------------------\
  TreeBlockRow 0 28                              29
                 |---------------\               |---------------\
  TreeBlockRow 0 24              25              26              27
                 |-------\       |-------\       |-------\       |-------\
  TreeBlockRow 0 16      17      18      19      20      21      22      23
                 |---\   |---\   |---\   |---\   |---\   |---\   |---\   |---\
  TreeBlockRow 0 00  01  02  03  04  05  06  07  08  09  10  11  12  13  14  15
```

TreeBlocks aren't stored individually but are grouped together which form a TreeTable.

### TreeTable

TreeBlocks are grouped into TreeTables and is accessed and written to disk as part of a TreeTable.

TreeTables must only have TreeBlocks with same forestRows. This means that the 'TreeBlockRow 1' TreeBlock:

```
   em
   |---------------\
   em              em
   |-------\       |-------\
   em      em      em      em
   |---\   |---\   |---\   |---\
   00  em  em  em  em  em  em  em
```

cannot be in a same TreeTable as the other 2 TreeBlocks in the forest. So to represent the Utreexo Forest
of forestRows = 4 with treeBlocks with forestRows = 3, there needs to be 2 TreeTables.

TreeTables have a maximum size of:
   `[(Hash size) * (2 << forestRows of the TreeBlock) * (Number of TreeBlocks per TreeTable)] + metadata`

When written, each TreeTable is essentially immutable. They are never overwritten.
All modifications to the treeTable are done in-memory. For a modification, a TreeTable is
loaded onto memory and done there. The loaded TreeTable is given a new file number and the old
is saved as stale for later garbage collection.

A change to the disk only happens during a commit. In a commit, new TreeTables are redirected
onto new files and stale TreeTables are garbage collected.

As with TreeBlocks, individual TreeTables aren't aware of their position relevant to the entire
forest. Therefore, a data must be kept to keep track of which TreeTable holds which TreeBlocks.
This is kept in the manifest.

### Manifest

Manifest holds all neccessary data for loading a utreexo forest from disk. Manifests are only
overwritten when the cowForest commit was successful.

Manifest holds a 2d array called 'location' that save which on-disk .ufod file holds.

For the above Utreexo forest example, if there were 2 TreeBlocks per TreeTable it may look like:

```
     		  (offset 0)    (offset 1)

(treeBlockRow 0)  [1.ufod]    	[nil]		stores row 0-3
(treeBlockRow 1)  [2.ufod] 	[nil]		stores row 4-7
```

The offset is calulcated by getting the treeBlockOffset and dividing it by the number of TreeBlocks
in a TreeTable.
