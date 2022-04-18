module github.com/mit-dci/utreexo

go 1.18

require (
	github.com/adiabat/bech32 v0.0.0-20170505011816-6289d404861d
	github.com/btcsuite/btcd v0.21.0-beta.0.20201124191514-610bb55ae85c
	github.com/btcsuite/btcutil v1.0.3-0.20201208143702-a53e38424cce
	github.com/dvyukov/go-fuzz v0.0.0-20210914135545-4980593459a1 // indirect
	github.com/syndtr/goleveldb v1.0.1-0.20200815110645-5c35d600f0ca
	golang.org/x/exp v0.0.0-20220317015231-48e79f11773a // indirect
)

replace github.com/btcsuite/btcd => github.com/mit-dci/utcd v0.21.0-beta.0.20210716180138-e7464b93a1b7
