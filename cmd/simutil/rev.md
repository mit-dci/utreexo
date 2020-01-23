# File format — rev*.dat

## rev*NNNNN*.dat

The *NNNNN* is a number starting from zero.

|Description|Data type|Comments|
|--|--|--|
|Records|BlockRecord[]|Data of each Block|

Maybe this file is less than 20MB in size.

## BlockRecord

|Description|Data type|Comments|
|--|--|--|
|Magic|byte[4]|Magic value , little endian|
|Size|byte[4]|'Block'(CBlockUndo) data size , little endian|
|Block|CBlockUndo|'Size' bytes of data|
|Hash|byte[32]|Double sha256 hash of data combining 'block hash' and 'Block' (CBlockUndo) data (*1)|

*1: https://github.com/bitcoin/bitcoin/blob/af05bd9e1e362c3148e3b434b7fac96a9a5155a1/src/validation.cpp#L1581

## CBlockUndo

|Description|Data type|Comments|
|--|--|--|
|Size|CompactSize|'Txs' data size|
|Txs|CTxUndo[]|Data of each Transaction in Block|


## CTxUndo

|Description|Data type|Comments|
|--|--|--|
|Size|CompactSize|'Ins' data size|
|Ins|Coin[]|Data of each Input in Transaction|

## Coin

|Description|Data type|Comments|
|--|--|--|
|Height|VarInt|2*height (+1 if it was a coinbase output)|
||byte|0x00 , Required to maintain compatibility with older undo format.|
|TxOut|CTxOut||

## CTxOut

|Description|Data type|Comments|
|--|--|--|
|Amount|VarInt(CompressAmount)|Amount compressed with CompressAmount and converted to VarInt|
|Script|CompressScript|Compressed script|

# Common data type

## CompactSize

https://github.com/bitcoin/bitcoin/blob/master/src/serialize.h#L270

```
Compact Size
size <  253        -- 1 byte
size <= USHRT_MAX  -- 3 bytes  (253 + 2 bytes)
size <= UINT_MAX   -- 5 bytes  (254 + 4 bytes)
size >  UINT_MAX   -- 9 bytes  (255 + 8 bytes)
```

## VarInt

https://github.com/bitcoin/bitcoin/blob/master/src/serialize.h#L344

```
Variable-length integers: bytes are a MSB base-128 encoding of the number.
The high bit in each byte signifies whether another digit follows. 
To make sure the encoding is one-to-one, one is subtracted from all but the last digit.
Thus, the byte sequence a[] with length len, where all but the last byte has bit 128 set, encodes the number:

 (a[len-1] & 0x7F) + sum(i=1..len-1, 128^i*((a[len-i-1] & 0x7F)+1))

Properties:
* Very small (0-127: 1 byte, 128-16511: 2 bytes, 16512-2113663: 3 bytes)
* Every integer has exactly one encoding
* Encoding does not depend on size of original integer type
* No redundancy: every (infinite) byte sequence corresponds to a list of encoded integers.

0:         [0x00]  256:        [0x81 0x00]
1:         [0x01]  16383:      [0xFE 0x7F]
127:       [0x7F]  16384:      [0xFF 0x00]
128:  [0x80 0x00]  16511:      [0xFF 0x7F]
255:  [0x80 0x7F]  65535: [0x82 0xFE 0x7F]
2^32:           [0x8E 0xFE 0xFE 0xFF 0x00]
```

## CompressAmount

https://github.com/bitcoin/bitcoin/blob/master/src/compressor.cpp#L140

```
Amount compression:
* If the amount is 0, output 0
* first, divide the amount (in base units) by the largest power of 10 possible; call the exponent e (e is max 9)
* if e<9, the last digit of the resulting number cannot be 0; store it as d, and drop it (divide by 10)
  * call the result n
  * output 1 + 10*(9*n + d - 1) + e
* if e==9, we only know the resulting number is not zero, so output 1 + 10*(n - 1) + 9
(this is decodable, as d is in [1-9] and e is in [0-9])
```

## CompressScript

https://github.com/bitcoin/bitcoin/blob/master/src/compressor.h#L25

```
Compact serializer for scripts.

It detects common cases and encodes them much more efficiently.
3 special cases are defined:
* Pay to pubkey hash (encoded as 21 bytes)
* Pay to script hash (encoded as 21 bytes)
* Pay to pubkey starting with 0x02, 0x03 or 0x04 (encoded as 33 bytes)

Other scripts up to 121 bytes require 1 byte + script length.
Above that, scripts up to 16505 bytes require 2 bytes + script length.
```

```
make this static for now (there are only 6 special scripts defined)
this can potentially be extended together with a new nVersion for transactions, in which case this value becomes dependent on nVersion and nHeight of the enclosing transaction.
```

### P2PKH(Pay-to-PubkeyHash)

- **scriptPubKey :** `OP_DUP OP_HASH160 <pubKeyHash> OP_EQUALVERIFY OP_CHECKSIG`
- **compressScript :** `0x00 <pubKeyHash>`

  **Size :** 21 bytes

### P2SH(Pay-to-Script-Hash)

- **scriptPubKey :** `OP_HASH160 <scriptHash> OP_EQUAL`
- **compressScript :** `0x01 <scriptHash>`

  **Size :** 21 bytes

### P2PK(Pay-to-Pubkey)

- **scriptPubKey :** `<pubKey> OP_CHECKSIG`
- **compressScript :** `<0x02 or 0x03 or 0x04 or 0x05> <pubkey x coordinate (32 bytes)>`

  **Size :** 33 bytes

If &lt;pubKey&gt; of 'scriptPubKey' is in compressed format, the first byte is 0x02 or 0x03.

If &lt;pubKey&gt; of 'scriptPubKey' is in uncompressed format, the first byte is 0x04 or 0x05.

The 0x04 means that y coordinate is even.

The 0x05 means that y coordinate is odd.

### Other

An Other is MultiSig , P2WPKH , P2WSH and so on.

- **scriptPubKey :** `<script>`
- **compressScript :** `<(script length) + 6> <script>`

  The `<(script length)> + 6>` is a **VarInt** type.

  **Size :** VarInt size of `<(script length) + 6>` + size of `<script>`


# References

- https://bitcoin.stackexchange.com/questions/57978/file-format-rev-dat

- https://github.com/bitcoin/bitcoin

