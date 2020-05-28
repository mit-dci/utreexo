#!/bin/bash
# Integration test script for the csn and bridge node.
# Assumes that `the following binaries exist:
# 	`bitcoind`, `bitcoin-cli, `utreexo`

set -Eeuo pipefail

TEST_DATA=$(mktemp -d)
BITCOIN_DATA=$TEST_DATA/.bitcoin
BITCOIN_CONF=$BITCOIN_DATA/bitcoin.conf
# number of blocks with dummy transactions in them
# increase this to generate more utxos
BLOCKS=10
# coins send to this address won't be spend
# used to create long lasting UTXOs
UNSPENDABLE_ADDR="bcrt1qduc2gmuwkun9wnlcfp6ak8zzphmyee4dakgnlk"

timestamp() {
	date "+%Y-%m-%d %H:%M:%S"
}

log() {
	echo "$(timestamp) $1"
}

# prints "faiL: $1" to stderr and exits if $? indicates an error.
failure() {
	echo "$(timestamp) $(tput setaf 1)FAILURE$(tput sgr0) on line ${BASH_LINENO[0]}: test data(logs, regtest files, ...) can be found here: $TEST_DATA" >&2
	# stop bitcoin core if its running but ignore if this fails.
	bitcoin-cli -conf=$BITCOIN_CONF stop > /dev/null 2>&1 || true
	# print genproofs and ibdsim logs
	tail $TEST_DATA/genproofs.log $TEST_DATA/ibdsim.log 2> /dev/null || true
}

trap "failure" ERR
trap "kill 0" EXIT

# mines n=$1 blocks
mine_blocks() {
	local mine_addr=$(bitcoin-cli -conf=$BITCOIN_CONF getnewaddress)
	bitcoin-cli -conf=$BITCOIN_CONF generatetoaddress $1 $mine_addr > /dev/null
}

# creates UTXOs/sends transactions
# 50% of these UTXOs will not be spend by future blocks.
# TODO: find a better way to create lots of utxos
create_utxos() {
	#outputs=$(cat unspendable_outputs.json)
	for ((i=0;i<10;i++))
	do
		if (("$i" < 5))
		then
			bitcoin-cli -conf=$BITCOIN_CONF sendtoaddress "$(bitcoin-cli -conf=$BITCOIN_CONF getnewaddress)" 0.01 > /dev/null &
		else
			bitcoin-cli -conf=$BITCOIN_CONF sendtoaddress "$UNSPENDABLE_ADDR" 0.01 > /dev/null &
		fi
	done
	wait
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
log "Waiting for bitcoind to start"
sleep 5

# mine 200 blocks to have u bunch of spendable coins
mine_blocks 200

# create blocks, with some transactions in them
log "Creating $BLOCKS blocks"
for ((n=0;n<$BLOCKS;n++))
do
	create_utxos
	mine_blocks 1
	log "Mined block $n"
done

log "Done creating blocks"
bitcoin-cli -conf=$BITCOIN_CONF gettxoutsetinfo

# stop bitcoin-core
bitcoin-cli -conf=$BITCOIN_CONF stop

# run genproofs
log "running genproofs..."
(cd $BITCOIN_DATA/regtest/blocks; utreexo genproofs -net=regtest > $TEST_DATA/genproofs.log 2>&1) &

log "waiting for genproofs to start the blocks server..."
while :
do
	if jobs %% > /dev/null 2>&1; then
		nc localhost 8338 < /dev/null && break
	else
		# genproofs failed => exit
		exit 1
	fi
	sleep 1
done
log "genproofs started the block server"

# run ibdsim
log "running idbsim..."
(cd $BITCOIN_DATA/regtest/blocks; utreexo ibdsim -net=regtest > $TEST_DATA/ibdsim.log 2>&1)

echo "$(timestamp) $(tput setaf 2)SUCCESS$(tput sgr0): everything seems to be working. test data(logs, regtest files, ...) can be found here: $TEST_DATA"