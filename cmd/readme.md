# cmd

Everything in cmd/ is related to the actual node implementation of Utreexo.

## csn

Implements the Utreexo Compact State Node. The CSN is the node that keeps only
the Utreexo tree tops. For caching purposes, some TXOs may be kept. However, when
flushing to disk, the cache data isn't flushed. This feature will come in the future.

## bridgenode

Since a Bitcoin Core node cannot serve Utreexo proofs, a bridge node is needed.
The bridge node currently does:

1. Generate an index from the provided blk*.dat files.
2. Generate TXO proofs.
3. Do the Utreexo accumulator operations and maintain an Utreexo Forest.

The bridge node currently does not:

1. Verify headers
2. Verify blocks
3. Verify signatures

The general idea for a bridge node is outlined in Section 4.5 in the Utreexo paper.
https://github.com/mit-dci/utreexo/blob/master/utreexo.pdf

## ttl

Time To Live is the representation of how long each transaction "lives" until it is
spent.

For example, a transaction that is created in block #200 and spent at block #400 has
a ttl value of 200.

This is needed for caching outlined in Section 5.3 and 5.4 in the Utreexo paper.
https://github.com/mit-dci/utreexo/blob/master/utreexo.pdf

## util

Various reused functions, constants, and paths used in all the packages.
