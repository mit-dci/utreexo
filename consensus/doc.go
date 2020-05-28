/*
This package outlines the consensus code that the Utreexo CSN node follows.
There are no plans to make one for the bridgenode as it is taking block data
that has already been verified by bitcoind. However, this is trivial to do from
the exisiting code here.

Verification isn't dependent on the database which is a nice side-effect.

Utreexo adds an additional step for tx verification. Before the transaction
scripts and amounts can be verified, the existence has to be verified.

1) P.ProofVerify()

The basic flows go likes such:

1) CSN recevies a block from bridgenode or another CSN with Utreexo proofs.
2) Block Headers are verified. Abort if verification fails.
3) Utreexo proofs are verified. Existence of the txs are verified here.
   Abort if verification fails.
4) Individual tx scripts and amounts are verified.

// LeafData is all the data that goes into a leaf in the utreexo accumulator
// A leaf is the bottommost node in a Utreexo tree. It represents a Bitcoin UTXO

	type LeafData struct {
	        BlockHash [32]byte
	        Outpoint  wire.OutPoint
	        Height    int32
	        Coinbase  bool
	        Amt       int64
	        PkScript  []byte
	}

Data needed for CSN tx verification:

1) LeafData
2) UData
3)

*/
package consensus
