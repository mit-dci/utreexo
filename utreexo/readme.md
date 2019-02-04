# the utreexo library

## overview

There are two main structs that do all the work: Forest and Pollard.  The Forest contains the entire utreexo accumulator structure and all data in it, and can be used to produce proofs and perform bridge node functions.  Pollard contains a partially populated accumulator and can verify proofs from the Forest (and should be able to produce some of its own).  A Pollard *can* contain the entire accumulator, but a Forest will be more efficient at doing so.  A Forest *must* contain everything.

## flow

Forest can be initialized and then have Modify() called repeatedly with hashes to add and *positions* to delete.  If you need to remember the positions of the things to delete based on their hashes, ProveBlock() will provide that data.

The general flow for pollards will be as in pollard_test.go.  A block proof is received by the pollard node, and IngestBlockProof() is called.  This populates the Pollard with data needed to remove everything that has been proved.  Then Modify() is called, with a list of things to delete and things to add.  (This two step process could be merged into 1 function call, and would be a bunch faster / more efficient, but for now it's 2 separate functions)

