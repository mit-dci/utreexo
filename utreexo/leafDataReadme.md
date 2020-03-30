## Leaf hashes and DB for utreexo


### leaf hash

Each leaf contains a UTXO.  The serialization is as follows:

leafhash = h( BH, OP, CH, AMT, PKS )

Where "," is concatenating the bytes together.  

BH:  block hash, 16 bytes.  The final 16 bytes of the sha256d of the block where this utxo was created.  (The initial 16 bytes are mostly 0s and are omitted) 

OP: outpoint, 36 bytes.  32 byte TXID (sha256d of the serialized transaction) then 4 byte output index

CH: coinbase height.  4 bytes, block height but with 1 bit for if it's a coinbase or not.  As with bitcoin core, check LSB for coinbaseness, and >> 1.

AMT: Amount, 8 bytes.  How many satoshis.

PKS: The whole pubkey script, as it shows up in the output.  This is the only variable length element, and is at the end so the start positions of all the elements are fixed.

### Bridge DB

The bridge node needs to store not just the full forest, but also a lookup database of leaf hash preimage data.  This is a key:value store that can be 
done with hashmaps, or in this case levelDB. 

The key is the 36 byte outpoint, and the value is the "posdata", defined as:

(POS, CH, AMT, TPKS)

POS is the position within the forest.  Unfortunately this changes a bunch, so it's not write once read once; the postada for a utxo is going to be read, modified, and written back a number of times before deletion.

CH and AMT as as defined for the leaf hash, and TPKS is a tagged pubkey script.  If it starts with 0xff, that means the following bytes are the raw 
pubkey script.  There can also be "tags" where in many cases we don't have to store the full pubkey script.  For example, if the UTXO is P2PKH (which most 
are) we can just store the fact that it's P2PKH, and observe the pubkey from the spending input to reconstruct the utxo's script.  If the wrong key is 
provided, the inclusion proof will fail, which is a different failure than the script failing, but this seems OK since no matter what it fails.

### sequence of operations:

Block comes in.  Bridge node makes sure the block is ok (normal verification) before modifying it's db and building proofs.  If not, abort.

First, UTXO deletion.

Bridge node loops through every input in the block, and queries every OP, getting posdata from the DB.  It gets the position, which it uses to build the proof tree.  It also gets the rest of the posdata.  It then sends posdata along with the proof sparse forest.

The compact state node receives a block proof, which consists of the sparse forest, and sequence of posdata for each target utxo.  The targets are sent in the order of the inputs in the block.

The CSN can reconstruct the leafdata for each target.  Leafdata is h( BH, OP, CH, AMT, PKS ).  

It gets CH, AMT, and TPKS from the block proof.  It gets BH by looking up the blockhash from CH.  It gets OP from the block itself.  Hashing all that together, it can then verify the sparse forest with respect to its stored sparse forest.  Once that verifies OK, it can use the posdata to run script and signature verification.

Next, UTXO additions.

The bridge node and compact state node do the same thing to the forest for additions; the bridge node just needs to do some additional DB writes.  Compute leafhash for every new output in the block (all the info is right there in the outputs and the blockhash), and add them to the forest.

For the bridge node, compute the posdata and add to the database.  Key: OP, value: posdata.  Straightforward enough; adds are always the easy part in utreexo, in contrast to money in general where it's a lot easier to spend it than make it.
