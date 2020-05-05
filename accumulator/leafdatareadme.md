# Leaf hashes and DB for utreexo

A leaf is the bottom most row and the elements that are added to the Utreexo
accumulator. This leaf is represented as a hash.

The utreexo Bridge node serves an important role in suppling the CSNs with
proofs.

### Leaf Hash

Each leaf contains a UTXO and the serialization is as follows:

Utxodata = ( CH, AMT, PKS )
Leafdata = ( BH, OP, Utxodata )
Leafhash = H( BH, OP, Utxodata )

Where "," is concatenating the bytes together.  

H: sha256 hash

BH: 16 byte block hash of the final 16 bytes of the sha256d (double sha hash)
of the block where this utxo was created. (The initial 16 bytes are mostly 0s
and are omitted)

OP: 36 bytes of the block where the tx is spent (OP stands for outpoint). This
is a concatenation of 32 byte TXID (sha256d of the serialized transaction) then
4 byte output index.

CH: 4 byte Coinbase height. This is a block height but with 1 bit for if it's a
coinbase or not. As with bitcoin core, check LSB for coinbaseness (CH & 1 == 1)

AMT: Amount, 8 bytes.  How many satoshis this tx transacts.

PKS: The whole pubkey script as it shows up in the output. This is the only
variable length element, and is at the end so the start positions of all the
elements are fixed.

### Bridge Node DB

The bridge node needs a key/value stores. This is:

1. Position DB - A mapping from Leafhash to Position within the forest.  

Leafhash (32 bytes) : Position (8 bytes)

Leafhash can probably be safely truncated. Position could also be 4 bytes since
we're well under 4G utxos, and will be for years.

# Sequence of Operations:

## Deletions

### Bridge Node

(Verify)
1. Bride node receives a block.
2. Verify the block. Abort if verification fails.

(Generate Proofs)
3. Make proofs for TXIN for the block.
4. Loop through every TXIN in the block and query the UTXO DB with OP:
	A. Utxodata
	B. BH
	C. and get block hashes and has Leafdata for every TXIN in the block.
5. Calculate Leafhash for every TXIN with the fetched information.

(Create Sparse Forest proof)
6. Query the Forest Position DB for the position of the leaves.
7. Build the proof tree with the position info.
8. Send batch proofs which consists of the following to CSN:
	A. leafdata
	B. position of each leafdata
	C. sparse forest

### Compact State Node

(Verify)
1. CSN receives a batch proof for TXINs which consists of
	A. leafdata
	B. sparse forest
Note that leafdata are sent in the order of the TXINs in the block.

Note: Bridge node can omit redundant parts of leafdata when sending over to the
CSN. BH and OP can be figured out for themselves. Brige node can also send
tagged pubkeys to save space there.
2. Hash all the individual batch proofs to Leafdata.
3. Recreate Sparse Forest in respect to its stored sparse forest.
4. Hash and compare the roots. Abort if verification fails. Continue elsewise.
5. Use posdata run txvalidation for each TXIN.

## Additions

Note: The processes for Bridge node and CSN are the same for additions.

(Compute)
1. Compute Leafhash for every new output in the block. During additions, all the
   info need to compute Leafhash is there.
2. Add one by one to the tree.

### Bridge Node
1. Maintain position data for each leaf in the position key/value store. This is
just for efficiency reasons.
