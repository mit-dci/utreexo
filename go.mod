module github.com/mit-dci/utreexo

go 1.12

require (
	github.com/btcsuite/btcd v0.20.1-beta
	github.com/btcsuite/btcutil v1.0.2
	github.com/golang/snappy v0.0.1 // indirect
	github.com/stevenroose/go-bitcoin-core-rpc v0.0.0-20181021223752-1f5e57e12ba1
	github.com/syndtr/goleveldb v1.0.0
	golang.org/x/crypto v0.0.0-20200622213623-75b288015ac9
	golang.org/x/sys v0.0.0-20200602225109-6fdc65e7d980 // indirect
)

replace github.com/btcsuite/btcd => github.com/rjected/btcd v0.0.0-20200602144129-70509ee4b219
