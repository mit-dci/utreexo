# blockparser

Blockparser reads bitcoin's blocks on disk, and outputs a text file with a text representation of all the utxos being created and destroyed.  This txos file can then be used in txottl and ibdsim to test utreexo performance.

## usage

Compile, and put the binary in ~/.bitcoin/testnet3/blocks for testnet or ~/.bitcoin/blocks for mainnet.  Alternatively copy blk*.dat to some folder and run it there.  You need to recompile for mainnet vs testnet.  It will generate a text file, which you can then feed into txottl.

TODO: merge blockparser, txottl, ibdsim all into one big binary.


