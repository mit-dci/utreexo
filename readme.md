# Utreexo

A dynamic hash based accumulator designed for the Bitcoin UTXO set

Check out the ePrint paper here: [https://eprint.iacr.org/2019/611](https://eprint.iacr.org/2019/611)

Currently under active development.  If you're interested and have questions, checkout #utreexo on freenode.

Logs for freenode are [here](http://gnusha.org/utreexo/)

---
## Importing Utreexo
The raw accumulator functions are in package accumulator. This is a general accumulator and is not bitcoin speicifc. For Bitcoin specific accumulator, look to the csn and bridgenode packages.

## Walkthrough for testing out Utreexo nodes

Here's how to get utreexo running to test out what it can do.  This currently is testing/research level code and should not be expected to be stable or secure.  But it also should work, and if it doesn't please report bugs!

To demonstrate utreexo we went with a client-server model. We have made prebuild binaries to run utreexo on Linux, Mac and Windows available here: https://github.com/mit-dci/utreexo/releases but you can also build from source.

### Client

#### Build from source
```
$ go get github.com/mit-dci/utreexo
$ cd ~/go/src/github.com/mit-dci/utreexo/cmd/utreexoclient
$ go build
```

#### Run
Running the client can take a couple of hours (There are still lots of performance optimisations to be done to speed things up).
The client downloads blocks with inclusion proofs from the server and validates them.
```
$ ./utreexoclient
[the client is able to resume from where it left off. Use ctrl+c to stop it.]
[To resume, just do `/utreexoclient` again]
```

*There is a `host` flag to specify a different server and a `watchaddr` flag to specify the address that you want to watch. To view all options use the `help` flag*

If you pause the client it will create the `pollardFile` which holds the accumulator roots. As an experiment you can copy this file to a different machine and resume the client at the height it was paused.

### Server
To try utreexo you do not need to run a server as we have a server set up for testing purposes which the client connects to by default. If you want to run your own server you can, see instructions below.

#### Build from source
```
$ go get github.com/mit-dci/utreexo
$ cd ~/go/src/github.com/mit-dci/utreexo/cmd/utreexoserver
$ go build
```

#### Run

If you want to run a server you will need the Bitcoin blockchain. Try testnet as it's smaller. (you can get Bitcoin Core from http://github.com/bitcoin/bitcoin or https://bitcoin.org/en/download)

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
**Note:** bitcoind has to be stopped before running the server.

The server should take a few hours. It does two things. First, it goes through the blockchain, maintains the full merkle forest, and saves proofs for each block to disk. Second, it saves each TXO and height with LevelDB to make a TXO time-to-live (basically how long each TXO lasts until it is spent) for caching purposes. This is what the bridge node and archive node would do in a real node.

```
$ cd ~/.bitcoin/testnet3/blocks
$ ./utreexoserver #path probably differs on your system
[... takes time and builds block proofs]
[the server is able to resume from where it left off. Use ctrl+c to stop it.]
[To resume, just do `./cmd genproofs -net=testnet` again]
```

After the server has generated the proofs, it will start a local server to serve the blocks to clients.

**Note**: your folders or filenames might be different, but this should give you the idea and work on default Linux/golang setups.  If you've tried this and it doesn't work and you'd like to help out, you can either fix the code or documentation so that it works and make a pull request, or open an issue describing what doesn't work.

### Windows walkthrough
<ol>
<li>
To run Utreexo, download the Bitcoin core here (includes both testnet and main chain): <a href="https://bitcoin.org/en/download ">https://bitcoin.org/en/download </a> Open the respective application; the testnet application should appear as a green bitcoin and the main bitcoin core application should appear as an orange bitcoin.
From here, synchronize with the blockchain; this takes around 2-5 hours on the testnet and up to a day using Bitcoin core.
</li>
<li>
Install Go in your pc and get it working on your compiler/IDE. The guide below will refer to installing Go on VSCode.
</li>
<li>

Get the Utreexo Code from ```github.com/mit-dci/utreexo```
</li>
<li>

Build and generate proofs (server) by running the following command where $USER is your username
```
go build bridgeserver.go
bridgeserver -datadir=C:\Users\$USER\AppData\Roaming\Bitcoin\testnet3\blocks\
```
 **If this fails, the command run was interupted or failed. To relaunch, delete the folders in the Utreexo\utreexoserver\utree folder: forestdata, offsetdata, pollarddata, proofdata, testnet-ttlbd**

</li>
<li>

 For a debugging session, in launch.json in VSCode, create a configuration as follows. The
      ```"-datadir"```    argument should point to the folder where Bitcoin was downloaded in step 1.

```
{
    "version": "0.2.0",
    "configurations": [
        {
            "name": "Launch",
            "type": "go",
            "request": "launch",
            "mode": "auto",
            "program": "${fileDirname}",
            "env": {},
            "args": ["-datadir=C:\\Users\\$USER\\AppData\\Roaming\\Bitcoin\\testnet3\\blocks\\"]
        }
    ]
}
```
</li>

<li>

Finally run Utreexo client from  **command line** using the following. Make sure that the server has finished running before running this command.

```
go build csnclient.go
C:\utreexo\csnclient -datadir=C:\Users\admin\AppData\Roaming\Bitcoin\testnet3\blocks\
```
</li>
</ol>
