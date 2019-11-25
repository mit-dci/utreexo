# utreexo

A dynamic hash based accumulator designed for the Bitcoin UTXO set

Check out the ePrint paper here: [https://eprint.iacr.org/2019/611](https://eprint.iacr.org/2019/611)

Currently under active development.  If you're interested and have questions, checkout #utreexo on freenode.

## folders

### cmd

subfolders with executable code

### utreexo

the utreexo libraries

### walkthrough

Here's how to get utreexo running to test out what it can do.  This currently is testing / research level code and should not be expected to be stable or secure.  But it also should work, and if it doesn't please report bugs!

---

* first, get the bitcoin blockchain.  Try testnet as it's smaller.  (you can get bitcoin core from github.com/bitcoin/bitcoin)

```
[ ...install bitcoin core ]
$ echo "testnet=1" > ~/.bitcoin/bitcoin.conf
$ bitcoind --daemon
[wait for testnet to sync]
$ du -h ~/.bitcoin/testnet3/blocks/
	214M	~/.bitcoin/testnet3/blocks/index
	24G	~/.bitcoin/testnet3/blocks/
[OK looks like it's there]
$ bitcoin-cli stop
```

* get utreexo code

```
$ go get github.com/mit-dci/utreexo
```

* build utreexo/simcmd/sim.go

This will give you simcmd binary.

simcmd contains various commands that go from organizing the .dat files from Bitcoin Core to actually doing a Utreexo simulation. To view all the available commands and flags, just run './simcmd' by itself.

First we need to organize the blocks in .dat file and build a giant ascii file of all transactions and record how long each transaction lasts until it is spent. 
```
$ cd ~/go/src/github.com/mit-dci/utreexo/cmd/simcmd/
$ go build
$ cp simcmd ~/.bitcoin/testnet3/blocks
$ cd ~/.bitcoin/testnet3/blocks
$ ./simcmd parseblock
[... takes some time, builds testnet.txos file and ttldb/ folder]
```

* Now there's testnet.txos, which is all the txs, and ttldb, which is the lifetimes of all utxos.  Now use txottlgen command to combine them into a single text file with txo lifetimes.

```
$ cd ~/.bitcoin/testnet3/blocks
$ ./simcmd txottlgen
[...takes time again]
```

* Now there's a ttl.testnet.txos file, which has everything we need.  At this point you can delete everything else (ttldb, testnet.txos, heck the whole blocks folder if you want).  Now you can run ibdsim to test utreexo sync performance.  First, the "genproofs" command builds all the block proofs for the blockchain.

```
$ cd ~/.bitcoin/testnet3/blocks
$ ./simcmd genproofs -ttlfn=ttl.testnet.txos // ttlfn flag is required for testnet
[... takes time and builds block proofs]
```

* "genproofs" should take a few hours as it goes through the blockchain, maintains the full merkle forest, and saves proofs for each block to disk.  This is what the bridge node and archive node would do in a real node.  Next, you can run 'simcmd ibdsim -ttlfn=ttl.testnet.txos'; it will perform IBD as a compact node which maintains only a reduced state, and accepts proofs (which are created in the `proofdb` folder during the previous step)


```
$ cd ~/.bitcoin/testnet3/blocks
$ ./simcmd ibdsim -ttlfn=ttl.testnet.txos // ttlfn flag is required for testnet
[... takes time and does utreexo sync simulation]
```

Note that your folders or filenames might be different, but this should give you the idea and work on default linux / golang setups.  If you've tried this and it doesn't work and you'd like to help out, you can either fix the code / documentation so that it does work and make a pull request, or open an issue describing what doesn't work.
