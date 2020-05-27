#!/bin/bash
# Integration test script for the csn and bridge node.
# WARNING: The bitcoin config at $BITCOIN_CONF will be overridden.
# WARNING: Regtest data at $BITCOIN_DATA/regtest will be deleted
# Assumes that `the following binaries exist:
# 	`bitcoind`, `bitcoin-cli, `utreexo`

BITCOIN_DATA=$HOME/.bitcoin
BITCOIN_CONF=$HOME/.bitcoin/bitcoin.conf
TIMESTAMP=`date "+%Y-%m-%d %H:%M:%S"`
# number of blocks with dummy transactions in them
# increase this to generate more utxos
BLOCKS=0

log() {
	echo "[$TIMESTAMP] $1"
}

# prints "faiL: $1" to stderr and exits if $? indicates an error.
print_on_failure() {
	if [ $? -ne 0 ]
	then
		bitcoin-cli -conf=$BITCOIN_CONF stop

		echo "[$TIMESTAMP] fail: $1" >&2
		exit 1
	fi
}

# mines n=$1 blocks
mine_blocks() {
	local mine_addr=$(bitcoin-cli -conf=$BITCOIN_CONF getnewaddress)
	bitcoin-cli -conf=$BITCOIN_CONF generatetoaddress $1 $mine_addr > /dev/null
	print_on_failure "could not mine blocks"
}

# creates utxos by sending coins to unspendable addresses.
# TODO: find a better way to create lots of utxos
create_utxos() {
	log "creating 1000 utxos"
	outputs=$(cat unspendable_outputs.json)
	bitcoin-cli -conf=$BITCOIN_CONF sendmany "" "$outputs" > /dev/null
}

# create/override a bitcoin config for this test
mkdir -p $BITCOIN_DATA
tee -a >${BITCOIN_CONF} <<EOF
daemon=1
regtest=1
server=1
fallbackfee=0.0002
[regtest]
rpcuser=utreexo
rpcpassword=utreexo
rpcport=8332
EOF

# remove old regtest
rm -rf $BITCOIN_DATA/regtest

# start bitcoin-core in regtest mode
bitcoind -datadir=$BITCOIN_DATA -conf=$BITCOIN_CONF
print_on_failure "could not start bitcoind."
log "Waiting for bitcoind to start"
sleep 5

# mine 101 bocks to have 50 spenable coins
mine_blocks 101

# create utxos, then mine a block over and over again
for ((n=0;n<$BLOCKS;n++))
do
	create_utxos
	mine_blocks 1
	log "Mined block $n"
done

bitcoin-cli -conf=$BITCOIN_CONF gettxoutsetinfo

# stop bitcoin-core
bitcoin-cli -conf=$BITCOIN_CONF stop
print_on_failure "could not stop bitcoind."

# run genproofs
(cd $BITCOIN_DATA/regtest/blocks; utreexo genproofs -net=regtest)
print_on_failure "genproofs failed"

# TODO: run ibdsim

