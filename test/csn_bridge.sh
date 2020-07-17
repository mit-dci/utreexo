#!/usr/bin/env bash
# Integration test script for the csn and bridge node.
# Assumes that `the following binaries exist:
# 	`bitcoind`, `bitcoin-cli`
#
# Receives the path to the genproofs and ibdsim command.
# Example:
#   ./csn_bridge.sh "./cmd genproofs" "./cmd ibdsim"
set -Eeuo pipefail

GENPROOFS="$1"
IBDSIM="$2"
TEST_DATA=$(mktemp -d)
BITCOIN_DATA=$TEST_DATA/.bitcoin
BITCOIN_CONF=$BITCOIN_DATA/bitcoin.conf
# number of blocks with dummy transactions in them
# increase this to generate more utxos
BLOCKS=10
# transactions per block
TX_PER_BLOCK=10
# transactions with unspendable outputs per block
UTX_PER_BLOCK=5
# coins send to this address won't be spend
# used to create long lasting UTXOs
UNSPENDABLE_ADDR="bcrt1qduc2gmuwkun9wnlcfp6ak8zzphmyee4dakgnlk"
GENPROOFS_TIMEOUT=60

# sadly `realpath` does not exist on macOS, so this is a lazy replacement.
# does not resolve .. in paths.
realpath() {
    [[ $1 = /* ]] && echo "$1" || echo "$PWD/${1#./}"
}

# checks that all required binaries exist
check_binaries() {
	log "checking if required binaries exist..."
	# check genproofs
	read -ra split <<< $GENPROOFS
	absolute_path=$(realpath ${split[0]})
	command -v $absolute_path
	split[0]=$absolute_path
	GENPROOFS=$(echo ${split[@]})

 	# check ibdsim
	read -ra split <<< $IBDSIM
	absolute_path=$(realpath ${split[0]})
	command -v $absolute_path
	split[0]=$absolute_path
	IBDSIM=$(echo ${split[@]})

	# check bitcoind
	command -v bitcoind
	#	check bitcoin-cli
	command -v bitcoin-cli
	# check netcat
	command -v nc
	log "all binaries found"
}

timestamp() {
	date "+%Y-%m-%d %H:%M:%S"
}

log() {
	echo "$(timestamp) $1"
}

failure() {
	echo "$(timestamp) FAILURE on line ${BASH_LINENO[0]}: test data(logs, regtest files, ...) can be found here: $TEST_DATA" >&2
	# stop bitcoin core if its running but ignore if this fails.
	bitcoin-cli -conf=$BITCOIN_CONF stop > /dev/null 2>&1 || true
	# print genproofs and ibdsim logs
	tail $TEST_DATA/genproofs.log $TEST_DATA/ibdsim.log 2> /dev/null || true
	
	kill 0 > /dev/null 2>&1
}

success() {
	# stop bitcoin core if its running but ignore if this fails.
	bitcoin-cli -conf=$BITCOIN_CONF stop > /dev/null 2>&1 || true
	
	log "SUCCESS: everything seems to be working. test data(logs, regtest files, ...) can be found here: $TEST_DATA"
}

trap "failure" ERR

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
	for ((i=0;i<$TX_PER_BLOCK;i++))
	do
		if (("$i" < $UTX_PER_BLOCK))
		then
			bitcoin-cli -conf=$BITCOIN_CONF sendtoaddress "$UNSPENDABLE_ADDR" 0.01 > /dev/null &
		else
			bitcoin-cli -conf=$BITCOIN_CONF sendtoaddress "$(bitcoin-cli -conf=$BITCOIN_CONF getnewaddress)" 0.01 > /dev/null &
		fi
	done
	wait
}

# create blocks, with some transactions in them
create_blocks() {
	log "Creating $BLOCKS blocks"
	for ((n=0;n<$BLOCKS;n++))
	do
		create_utxos
		mine_blocks 1
	done

	log "Done creating blocks"
	bitcoin-cli -conf=$BITCOIN_CONF gettxoutsetinfo
}

# runs genproof in the background and waits for it to start the block server.
# when the block server is running ibdsim is started.
# prints the output of both if everything succeeds.
run_utreexo() {
	# run genproofs
	log "running genproofs..."
	eval "$GENPROOFS -datadir=$BITCOIN_DATA/regtest/blocks -net=regtest > $TEST_DATA/genproofs.log 2>&1 &"
	genproofs_id=$!

	log "waiting for genproofs to start the blocks server..."
	local sleep_counter=0
	while :
	do
		if [[ "$sleep_counter" -gt "$GENPROOFS_TIMEOUT" ]]; then
			log "timeout reached while waiting for genproofs to start the blocks server! failing..."
			false
		fi
		
		if jobs %% > /dev/null 2>&1; then
			nc -z localhost 8338 && break
		else
			# genproofs failed => exit
			wait $genproofs_id
		fi
		sleep 1
		((sleep_counter=$sleep_counter+1))
	done
	log "genproofs started the block server"

	# run ibdsim
	log "running idbsim..."
	eval "$IBDSIM > $TEST_DATA/ibdsim.log 2>&1"
	kill -SIGQUIT $genproofs_id > /dev/null 2>&1
	wait $genproofs_id

	log "utreexo output:"
	tail -n +1 $TEST_DATA/genproofs.log $TEST_DATA/ibdsim.log
}

check_binaries

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

# mine 200 blocks to have a bunch of spendable coins
mine_blocks 200

cd $BITCOIN_DATA/regtest/blocks
create_blocks

# stop bitcoin core to enable access to /index
bitcoin-cli -conf=$BITCOIN_CONF stop > /dev/null 2>&1 || true
sleep 1

# run utreexo
run_utreexo

success