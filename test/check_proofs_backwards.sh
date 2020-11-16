#!/usr/bin/env bash

GENPROOFS="$1"
DATA="$2"

timestamp() {
	date "+%Y-%m-%d %H:%M:%S"
}

log() {
	echo "$(timestamp) $1"
}

compare_proofs_backwards() {
	log "comparing proofs..."

	OLD_PROOFS=$(mktemp -d)
	eval "$GENPROOFS -datadir=$DATA/.bitcoin/ -net=regtest -bridgedir=$OLD_PROOFS -quitat=200 -noserve > /dev/null"

	if cmp -s $OLD_PROOFS/regtest/proofdata/proof.dat $DATA/currentproofs/regtest/proofdata/proof.dat; then
		log "proofs match up"
	else
		log "Proof mismatch"
		false
	fi
}

compare_proofs_backwards
