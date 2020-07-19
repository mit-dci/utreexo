module github.com/mit-dci/utreexo

go 1.12

require (
	github.com/adiabat/bech32 v0.0.0-20170505011816-6289d404861d
	github.com/btcsuite/btcd v0.20.1-beta
	github.com/btcsuite/btcutil v1.0.2
	github.com/golang/snappy v0.0.1 // indirect
	github.com/syndtr/goleveldb v1.0.0
	golang.org/x/crypto v0.0.0-20200604202706-70a84ac30bf9
	golang.org/x/sys v0.0.0-20200602225109-6fdc65e7d980 // indirect
)

replace github.com/btcsuite/btcd => github.com/rjected/btcd v0.0.0-20200718165331-907190b086ba
