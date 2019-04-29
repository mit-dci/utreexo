# cmd

folders with executables

General flow to get data on utreexo performance:

1. run blockparser to get txo list and ttl data.

2. run txottl to merge the ttl data into the txo file

3. run ibdsim -genroofs to build a db of proofs

4. run ibdsim to run through the IBD process with those proofs and see how much data & time it takes


## blockparser

goes through bitcoind's blocks folder and reads all the transaction data.  Creates 2 things: a txos text file, and a ttl leveldb folder. 

## txottl

txottl parses a text list of transactions and builds a database of how long txos last.  It can then append the txo durations to the transaction file.


## ibdsim

Performs the accumulator operations of initial block download (IBD) to measure performance.
Note that this doesn't do any signature or transaction verification, only the accumulator operations.

