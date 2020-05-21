package csn

import (
	"fmt"

	"github.com/btcsuite/btcd/wire"
	"github.com/mit-dci/utreexo/accumulator"
)

/*

Implementation of the ChainHook interface, which should be enough to
plug a wallet into.

type ChainHook interface {

	Start(height int32, host, path string, proxyURL string, params *coinparam.Params) (
		chan lnutil.TxAndHeight, chan int32, error)
	RegisterAddress(address [20]byte) error
	RegisterOutPoint(wire.OutPoint) error
	UnregisterOutPoint(wire.OutPoint) error
	PushTx(tx *wire.MsgTx) error
	RawBlocks() chan *wire.MsgBlock
}

Also, what's an "address"...?  Guess use whatever btcd uses.  Or just []byte,
but that's annoying because you might not have a 1:1 mapping with a PkScript...

When it starts, it should return 2 channels: a height channel of int32s which
just sends a number which pretty much just ticks up each block, and a channel of
txs, which come *before* the height where they're confirmed.  Could also have
a separate unconfirmed TX channel if there's interest in that...e

*/

// CsnHook is the main stateful struct for the Compact State Node.
// It keeps track of what block its on and what transactions it's looking for
type Csn struct {
	CurrentHeight int32
	pollard       accumulator.Pollard

	WatchOPs  map[wire.OutPoint]bool
	WatchAdrs map[[20]byte]bool
	// TODO use better addresses, either []byte or something fancy
	TxChan     chan wire.MsgTx
	HeightChan chan int32
}

func (ch *Csn) RegisterOutPoint(op wire.OutPoint) {
	ch.WatchOPs[op] = true
}

func (ch *Csn) UnegisterOutPoint(op wire.OutPoint) {
	delete(ch.WatchOPs, op)
}

func (ch *Csn) RegisterAddress(adr [20]byte) {
	ch.WatchAdrs[adr] = true
}

// TODO implement.  I guess push to the bridge node.  But really it'd
// be better to just push it out to the regular p2p network.
func PushTx(tx *wire.MsgTx) error {
	fmt.Printf("no PushTx yet\n")
	return nil
}
