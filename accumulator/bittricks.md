# Bit tricks

There are various bitwise operations that can be used to compute positions of relatives of a node in the forest.

**Example tree with 8 leaves (15 nodes) and forestRows=3:**
```
         decimal:                          binary:
row 3:   14                                1110
         |---------------\                 |-----------------------\
row 2:   12              13                1100                    1101
         |-------\       |-------\         |-----------\           |-----------\
row 1:   08      09      10      11        1000        1001        1010        1011
         |---\   |---\   |---\   |---\     |-----\     |-----\     |-----\     |-----\
row 0:   00  01  02  03  04  05  06  07    0000  0001  0010  0011  0100  0101  0110  0111
```

## Placement
The placement (left=0, right=1) of a node's position is indicated by its least significant bit (LSB 0). The LSB 1 indicates the parent's placement, the LSB 2 indicates the placement of the grandparent, ..., LSB n indicates the placement of the nth ancestor but only if n is smaller than the row index of the node.

## Children
Given a position `a` the left child can be computed with `(a << 1) & mask` where `mask` is the maximum number of nodes in the forest which can be calculated with `uint64(2<<forestRows) - 1`. The left shift promotes all bits in `a` to indicate the ancestor placements of the left child and the bitwise AND makes sure that the child position is smaller than `a`.  
The position of the `n`th descendant can be computed with `(a << n) & mask`.

## Siblings
Given a position `a` the sibling can be computed with `a^1` (`a` XOR 1). The XOR flips the LSB and therefore results in the position of the sibling of `a`.

In some cases you will see `a|1` (`a` OR 1) which returns the right sibling of `a` regardless of the placement of `a`. This is useful when dealing with sorted lists of positions because it lets you figure out if two positions are siblings quite easily.

## Cousins
Given a position `a` the cousin can be computed with `a^2` (`a` XOR 2). The XOR flips the second bit to compute the cousin because the second bit indicates the placement of the parent. The placement of the cousin will be the same as `a`.

## Parents

Given a position `a` the parent can be computed with `(a >> 1) | (1 << forestRows)`. The right shift of `a` removes the LSB 0 (placement bit) of `a` and the OR raises the position by one row.