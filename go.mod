module github.com/mit-dci/utreexo

go 1.12

require (
	github.com/adiabat/bech32 v0.0.0-20170505011816-6289d404861d
	github.com/btcsuite/btcd v0.21.0-beta.0.20201124191514-610bb55ae85c
	github.com/btcsuite/btcutil v1.0.3-0.20201208143702-a53e38424cce
	github.com/syndtr/goleveldb v1.0.1-0.20200815110645-5c35d600f0ca
	golang.org/x/tools v0.1.7
)

replace github.com/btcsuite/btcd => github.com/mit-dci/utcd v0.21.0-beta.0.20210716180138-e7464b93a1b7
