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

* build blockparser and run it in the bitcoin blocks folder to build a giant text file of all transactions

```
$ cd ~/go/src/github.com/mit-dci/utreexo/cmd/blockparser
$ go build
$ cp blockparser ~/.bitcoin/testnet3/blocks
$ cd ~/.bitcoin/testnet3/blocks
$ ./blockparser
[... takes some time, builds testnet.txos file and ttldb folder]
```

* Now there's testnet.txos, which is all the txs, and ttldb, which is the lifetimes of all utxos.  Use txottl to combine them into a single text file with txo lifetimes.

```
$ cd ~/go/src/github.com/mit-dci/utreexo/cmd/txottl
$ go build
$ cp txottl ~/.bitcoin/testnet3/blocks
$ cd ~/.bitcoin/testnet3/blocks
$ ./txottl
[...takes time again]
```

* Now there's a ttl.testnet.txos file, which has everything we need.  At this point you can delete everything else (ttldb, testnet.txos, heck the whole blocks folder if you want).  Now you can run ibdsim to test utreexo sync performance.  First, the "genproofs" flag of ibdsim builds all the block proofs for the blockchain

```
$ cd ~/go/src/github.com/mit-dci/utreexo/cmd/ibdsim
$ go build
$ cp ibdsim ~/.bitcoin/testnet3/blocks
$ cd ~/.bitcoin/testnet3/blocks
$ ./ibdsim -genproofs
[... takes time and builds block proofs]
```

* "genproofs" should take a few hours as it goes through the blockchain, maintains the full merkle forest, and saves proofs for each block to disk.  This is what the bridge node and archive node would do in a real node.  Next, you can run ibdsim without the "genproofs" flag; by default it will perform IBD as a compact node which maintains only a reduced state, and accepts proofs (which are created in the `proofdb` folder during the previous step)


```
$ cd ~/go/src/github.com/mit-dci/utreexo/cmd/ibdsim
$ go build
$ cp ibdsim ~/.bitcoin/testnet3/blocks
$ cd ~/.bitcoin/testnet3/blocks
$ ./ibdsim
[... takes time and does utreexo sync simulation]
```

Note that your folders or filenames might be different, but this should give you the idea and work on default linux / golang setups.  If you've tried this and it doesn't work and you'd like to help out, you can either fix the code / documentation so that it does work and make a pull request, or open an issue describing what doesn't work.

Also, if you think this setup is overly complicated and too many steps, I agree!  Couldn't blockparset and txottl be merged into one binary?  Sure could!  Maybe IBDsim could absorb that as well!  There's lots of stuff to work on.  (As of October 2019 I'm working on reorgs / undoing blocks)
  
