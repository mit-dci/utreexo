A dynamic hash-based accumulator.

Overview
--------

Package accumulator provides a general purpose dynamic accumulator. There are two main structs: Forest and Pollard.

The Forest contains the entire utreexo accumulator (all the nodes in the forest), and can be used to produce inclusion-proofs for Pollards to verify. Pollard contains a partially populated accumulator and can verify inclusion-proofs from the Forest. A Pollard *can* contain the entire accumulator. A Forest *must* contain everything.

Installation
------------

To install, just run the below command. Then the package will be ready to be linked in your Go code.

`go get github.com/mit-dci/utreexo/accumulator`

Usage
-----

To initialize a pollard or forest:

```
	// inits a Forest in memory. Refer to the documentation for NewForest() for an in-detail explanation
        // of all the different forest types.
	forest := accumulator.NewForest(nil, false, "", 0)

	// declare pollard. No init function for pollard
	var pollard accumulator.Pollard
```

To add/delete elements from the set:

```
	// Adds and Deletes leaves from the Forest
        // undoBlock is the data needed to revert the Modify. You can ignore it there's no
        // need to support rollbacks.
	undoBlock, err := forest.Modify(leavesToAdd, leavesToDelete)

	// Adds and Deletes leaves from the Pollard
	err := pollard.Modify(leavesToAdd, leavesToDelete)
```

To create an inclusion-proof (only forest is able to do this):

```
        // leavesToProve is a slice of leaves. To prove just one element, just have a slice with a single
        // element. ex: [ 1 ].
	proof, err := forest.ProveBatch(leavesToProve)
```

To verify the inclusion-proof (only implemented for pollard):

```
        // IngestBatchProof() first verifies the proof and returns an error if the proof is invalid.
        // It then readies the pollard for Modify() by populating the pollard which the proofs.
	err := pollard.IngestBatchProof(proof)
```

Documentation
-------------

You can read package documentation [here](https://pkg.go.dev/github.com/mit-dci/utreexo/accumulator).

IRC
---

We're on irc.libera.chat #utreexo.
