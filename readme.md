# utreexo

a dynamic hash based accumulator designed for the Bitcoin UTXO set

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
$ go get github.com/chainsafe/utreexo
```

* build blockparser and run it in the bitcoin blocks folder to build a giant text file of all transactions

```
$ cd ~/go/src/github.com/chainsafe/utreexo/tooling/blockparser
$ go build
$ cp blockparser ~/.bitcoin/testnet3/blocks
$ cd ~/.bitcoin/testnet3/blocks
$ ./blockparser
[... takes some time, builds testnet.txos file and ttldb folder]
```

* Now there's testnet.txos, which is all the txs, and ttldb, which is the lifetimes of all utxos.  Use txottl to combine them into a single text file with txo lifetimes.

```
$ cd ~/go/src/github.com/chainsafe/utreexo/tooling/txottl
$ go build
$ cp txottl ~/.bitcoin/testnet3/blocks
$ cd ~/.bitcoin/testnet3/blocks
$ ./txottl
[...takes time again]
```

* Now there's a ttl.testnet.txos file, which has everything we need.  At this point you can delete everything else (ttldb, testnet.txos, heck the whole blocks folder if you want).  Now you can run ibdsim to test utreexo sync performance.

```
$ cd ~/go/src/github.com/chainsafe/utreexo/tooling/ibdsim
$ go build
$ cp ibdsim ~/.bitcoin/testnet3/blocks
$ cd ~/.bitcoin/testnet3/blocks
$ ./ibdsim
[... takes time but does utreexo sync simulation]
```

Note that your folders or filenames might be different, but this should give you the idea and work on default linux / golang setups.  If you've tried this and it doesn't work and you'd like to help out, you can either fix the code / documentation so that it does work and make a pull request, or open an issue describing what doesn't work.  Thanks!
