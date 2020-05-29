# utreexo

A dynamic hash based accumulator designed for the Bitcoin UTXO set

Check out the ePrint paper here: [https://eprint.iacr.org/2019/611](https://eprint.iacr.org/2019/611)

Currently under active development.  If you're interested and have questions, checkout #utreexo on freenode.

Logs for freenode are [here](http://gnusha.org/utreexo/)

### walkthrough

Here's how to get utreexo running to test out what it can do.  This currently is testing/research level code and should not be expected to be stable or secure.  But it also should work, and if it doesn't please report bugs!

---

* first, get the Bitcoin blockchain.  Try testnet as it's smaller.  (you can get Bitcoin Core from http://github.com/bitcoin/bitcoin)

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

This will give you the `cmd` binary.

`cmd` contains various commands that go from indexing the `blk*.dat` files from Bitcoin Core to building the Bridge Node and the Compact State Node. To view all the available commands and flags, just run `./cmd` by itself.

First we need to organize the blocks in the `.dat` files, build a proof file and a db that keeps record of how long each transaction lasts until it is spent.

First, the `genproofs` command builds all the block proofs for the blockchain and the db for how long a transaction lasts.

```
$ cd ~/.bitcoin/testnet3/blocks
$ ./cmd genproofs -net=testnet # -net=testnet flag needed for testnet. Leave out for mainnet
[... takes time and builds block proofs]
[genproofs is able to resume from where it left off. Use ctrl+c to stop it.]
[To resume, just do `./cmd genproofs -net=testnet` again]
```

* `genproofs` should take a few hours. It does two things. First, it goes through the blockchain, maintains the full merkle forest, and saves proofs for each block to disk. Second, it saves each TXO and height with LevelDB to make a TXO time-to-live (basically how long each TXO lasts until it is spent) for caching purposes. This is what the bridge node and archive node would do in a real node.  Next, you can run `cmd ibdsim -net=testnet`; it will perform IBD (initial block download) as a compact node which maintains only a reduced state, and accepts proofs (which are created in the `proof.dat` file during the previous step).

After genproofs has generated the proofs, it will start a local server to serve the blocks to ibdsim. With genproofs running, run the following:

```
$ cd ~/.bitcoin/testnet3/blocks
$ ./cmd ibdsim -net=testnet # -net=testnet flag needed for testnet. Leave out for mainnet
[... takes time and does utreexo sync simulation]
[ibdsim is able to resume from where it left off. Use ctrl+c to stop it.]
[To resume, just do `./cmd ibdsim -net=testnet` again]
```

* `ibdsim` is the CSN node and it will call genproofs and ask for blocks with the Utreexo accumulator proofs. It will receive the proofs and validate the inclusion.

Note that your folders or filenames might be different, but this should give you the idea and work on default Linux/golang setups.  If you've tried this and it doesn't work and you'd like to help out, you can either fix the code or documentation so that it works and make a pull request, or open an issue describing what doesn't work.
