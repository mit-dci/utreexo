package mempool

import (
	"github.com/btcsuite/btcd/txscript"
)

type Mempool struct {
	Sigache   *txscript.SigCache
	HashCache *txscript.HashCache
}
