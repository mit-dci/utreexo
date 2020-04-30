## Leaf hashes and DB for utreexo


### leaf hash

Each leaf contains a UTXO.  The serialization is as follows:

utxodata = ( CH, AMT, PKS )
leafdata = ( BH, OP, utxodata )
leafhash = h( BH, OP, utxodata )

Where "," is concatenating the bytes together.  

BH:  block hash, 16 bytes.  The final 16 bytes of the sha256d of the block where this utxo was created.  (The initial 16 bytes are mostly 0s and are omitted) 

OP: outpoint, 36 bytes.  32 byte TXID (sha256d of the serialized transaction) then 4 byte output index

CH: coinbase height.  4 bytes, block height but with 1 bit for if it's a coinbase or not.  As with bitcoin core, check LSB for coinbaseness, and >> 1.

AMT: Amount, 8 bytes.  How many satoshis.

PKS: The whole pubkey script, as it shows up in the output.  This is the only variable length element, and is at the end so the start positions of all the elements are fixed.

### Bridge DB

The bridge node needs two key/value stores or databases.  The first is a mapping from outpoint to leafdata.  This mapping is just the UTXO set, so if the bridge node can query a bitcoin core full node or is itself a full node, it's got a DB to go from outpoints to utxo data.

The other mapping is from leafhash to position within the forest.  

UTXO DB - OP (36 bytes) : UTXO data (CH, AMT, PKS)

Position DB - leafhash (32 bytes) : position (8 bytes)

Leafhash can probably be safely truncated.  Position could also be 4 bytes since we're well under 4G utxos, and will be for years.

### sequence of operations:

Block comes in.  Bridge node makes sure the block is ok (normal verification) before modifying it's db and building proofs.  If not, abort.

First, making proofs for tx inputs.

Bridge node loops through every input in the block, and queries the UTXO DB for each of them.  It also queries the headers to get blockhashes, and has leafdata for every txin in the block.  It holds on to those, and also calculates leafhash for every txin.  It then queries the forest's position DB to get the position of the txins, which it uses to build the proof tree.  It then sends leafdata, position of each leaf, and the sparse forest proof.

The compact state node receives a block proof, which consists of the sparse forest, and sequence of leafdatas and positions for each target utxo.  The targets are sent in the order of the inputs in the block.  The bridge node can omit redundant parts of leafdata when sending over to the CSN: BH, OP.  (CSNs can figure those out for themselves)  It can also send tagged pubkeys to save space there.

The CSN can reconstruct the leafdata for each target.  (Leafdata is ( BH, OP, utxodata )).  

Hashing all that together, it can then verify the sparse forest with respect to its stored sparse forest.  Once that verifies OK, it can use the posdata to run script and signature verification.

Next, UTXO additions.

The bridge node and compact state node do the same thing to the forest for additions; the bridge node just needs to do some additional DB writes.  Compute leafhash for every new output in the block (all the info is right there in the outputs and the blockhash), and add them to the forest.

For the bridge node, maintain position data for each leaf in the position DB.  Also, add OP: utxodata to maintain the regular utxo set.  If attached to a normal full node this can be omitted, and can be looked up with gettxout.
