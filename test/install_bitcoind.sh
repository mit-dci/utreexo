#!/usr/bin/env bash

# Taken from https://github.com/lightningnetwork/lnd
# commit: 6e8021f8583bffe63c231c2c33d82cba0e17a53a

set -ev

export BITCOIND_VERSION=0.20.0

if sudo cp ~/bitcoin/bitcoin-$BITCOIND_VERSION/bin/bitcoind /usr/local/bin/bitcoind
then
        echo "found cached bitcoind"
else
        mkdir -p ~/bitcoin && \
        pushd ~/bitcoin && \
        wget https://bitcoin.org/bin/bitcoin-core-$BITCOIND_VERSION/bitcoin-$BITCOIND_VERSION-x86_64-linux-gnu.tar.gz && \
        tar xvfz bitcoin-$BITCOIND_VERSION-x86_64-linux-gnu.tar.gz && \
        sudo cp ./bitcoin-$BITCOIND_VERSION/bin/bitcoind /usr/local/bin/bitcoind && \
        popd
fi