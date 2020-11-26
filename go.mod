module github.com/mit-dci/utreexo

go 1.14

require (
	github.com/adiabat/bech32 v0.0.0-20170505011816-6289d404861d
	github.com/btcsuite/btcd v0.21.0-beta
	github.com/btcsuite/btclog v0.0.0-20170628155309-84c8d2346e9f
	github.com/btcsuite/btcutil v1.0.2
	github.com/btcsuite/go-socks v0.0.0-20170105172521-4720035b7bfd
	github.com/btcsuite/websocket v0.0.0-20150119174127-31079b680792
	github.com/golang/snappy v0.0.1 // indirect
	github.com/jessevdk/go-flags v1.4.0
	github.com/jrick/logrotate v1.0.0
	github.com/syndtr/goleveldb v1.0.0
	golang.org/x/crypto v0.0.0-20200604202706-70a84ac30bf9
	golang.org/x/sys v0.0.0-20200602225109-6fdc65e7d980 // indirect
)

//replace github.com/btcsuite/btcd => github.com/rjected/btcd v0.0.0-20201125090829-1e009a7ae42e
replace github.com/btcsuite/btcd => /home/calvin/bitcoin-projects/go/utreexo/go/src/github.com/btcsuite/btcd

replace github.com/btcsuite/btcutil => /home/calvin/bitcoin-projects/go/utreexo/go/src/github.com/btcsuite/btcutil

replace github.com/mit-dci/utreexo => /home/calvin/bitcoin-projects/go/utreexo/go/src/github.com/mit-dci/utreexo
