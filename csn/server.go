package csn

import (
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"time"

	"github.com/mit-dci/utreexo/accumulator"
	"github.com/mit-dci/utreexo/chain"
	"github.com/mit-dci/utreexo/wire"
)

// CsnHook is the main stateful struct for the Compact State Node.
// It keeps track of what block its on and what transactions it's looking for
type CompactState struct {
	pollard accumulator.Pollard
}

// UblockNetworkReader gets Ublocks from the remote host and puts em in the
// channel.  It'll try to fill the channel buffer.
func UblockNetworkReader(
	blockChan chan chain.UBlock, remoteServer string,
	curHeight, lookahead int32) {

	d := net.Dialer{Timeout: 2 * time.Second}
	con, err := d.Dial("tcp", remoteServer)
	if err != nil {
		panic(err)
	}
	defer con.Close()
	defer close(blockChan)

	// request range from curHeight to latest block
	err = binary.Write(con, binary.BigEndian, curHeight)
	if err != nil {
		e := fmt.Errorf("UblockNetworkReader: write error to connection %s %s\n",
			con.RemoteAddr().String(), err.Error())
		panic(e)
	}
	err = binary.Write(con, binary.BigEndian, int32(math.MaxInt32))
	if err != nil {
		e := fmt.Errorf("UblockNetworkReader: write error to connection %s %s\n",
			con.RemoteAddr().String(), err.Error())
		panic(e)
	}

	// TODO goroutines for only the Deserialize part might be nice.
	// Need to sort the blocks though if you're doing that
	for ; ; curHeight++ {
		var msgUBlock wire.MsgUBlock
		err = msgUBlock.Deserialize(con)
		if err != nil {
			fmt.Printf("Deserialize error from connection %s %s\n",
				con.RemoteAddr().String(), err.Error())
			return
		}

		ub := chain.NewUBlock(&msgUBlock)

		blockChan <- *ub
	}
}
