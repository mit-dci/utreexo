# utreexo

A dynamic hash based accumulator designed for the Bitcoin UTXO set

Check out the ePrint paper here: [https://eprint.iacr.org/2019/611](https://eprint.iacr.org/2019/611)

Currently under active development.  If you're interested and have questions, checkout #utreexo on freenode.

Logs for freenode are [here](https://github.com/utreexo-log/utreexo-irc-log)

## folders

### cmd

subfolders with implementation

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

* build utreexo

```
$ cd ~/go/src/github.com/mit-dci/utreexo/cmd/
$ go build
```

This will give you ``cmd``` binary

cmd contains various commands that go from indexing the blk*.dat files from Bitcoin Core to building the Bridge Node and the Compact State Node. To view all the available commands and flags, just run './cmd' by itself.

First we need to organize the blocks in .dat file, build a proof file and a db keeping record how long each transaction lasts until it is spent. 

First, the "genproofs" command builds all the block proofs for the blockchain and the db for keeping how long a transaction lasts.

```
$ cd ~/.bitcoin/testnet3/blocks
$ ./cmd genproofs -testnet=1 // -testnet=1 flag needed for testnet. Leave empty for mainnet
[... takes time and builds block proofs]
[genproofs is able to resume from where it left off. Use `ctrl + c` to stop it.]
[To resume, just do `./cmd genproofs -testnet=1 again`]
```

* "genproofs" should take a few hours as it does two things. First, it goes through the blockchain, maintains the full merkle forest, and saves proofs for each block to disk. Second, it saves each TXO and height with leveldb to make a TXO time to live (bascially how long each txo lasts until it is spent) for caching purposes. This is what the bridge node and archive node would do in a real node.  Next, you can run 'simcmd ibdsim -testnet=1'; it will perform IBD as a compact node which maintains only a reduced state, and accepts proofs (which are created in the proof.dat file during the previous step)


```
$ cd ~/.bitcoin/testnet3/blocks
$ ./cmd ibdsim -testnet=1 // -testnet=1 flag needed fro testnet. Leave empty for mainnet
[... takes time and does utreexo sync simulation]
[ibdsim is able to resume from where it left off. Use `ctrl + c` to stop it.]
[To resume, just do `./cmd ibdsim -testnet=1 again`]
```

Note that your folders or filenames might be different, but this should give you the idea and work on default linux / golang setups.  If you've tried this and it doesn't work and you'd like to help out, you can either fix the code / documentation so that it does work and make a pull request, or open an issue describing what doesn't work.
