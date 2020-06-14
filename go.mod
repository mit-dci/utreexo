module github.com/mit-dci/utreexo

go 1.12

require (
    github.com/btcsuite/btcd v0.20.1-beta
	github.com/golang/snappy v0.0.1 // indirect
	github.com/syndtr/goleveldb v1.0.0
	golang.org/x/crypto v0.0.0-20200604202706-70a84ac30bf9
	golang.org/x/sys v0.0.0-20200602225109-6fdc65e7d980 // indirect
)

replace github.com/btcsuite/btcd => github.com/rjected/btcd v0.0.0-20200602144129-70509ee4b219
