module github.com/mit-dci/utreexo

go 1.12

require (
	github.com/adiabat/bech32 v0.0.0-20170505011816-6289d404861d
	github.com/btcsuite/btcd v0.21.0-beta.0.20201124191514-610bb55ae85c
	github.com/btcsuite/btcutil v1.0.2
	github.com/golang/snappy v0.0.1 // indirect
	github.com/syndtr/goleveldb v1.0.0
	golang.org/x/crypto v0.0.0-20200604202706-70a84ac30bf9 // indirect
	golang.org/x/sys v0.0.0-20200602225109-6fdc65e7d980 // indirect
)

replace github.com/btcsuite/btcd => github.com/mit-dci/utcd v0.21.0-beta.0.20210201215500-359f1ee1429a
replace github.com/btcsuite/btcutil => github.com/mit-dci/utcutil v1.0.3-0.20210201144513-fb3ce8742498
