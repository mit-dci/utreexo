package bridgenode

import (
	"encoding/binary"
	"fmt"
	"net"

	"github.com/mit-dci/utreexo/util"
)

// blockServer listens on a TCP port for incoming connections, then gives
// ublocks blocks over that connection

func blockServer(endHeight int32) {
	for {
		fmt.Printf("starting UblockNetworkServer... ")
		err := UblockNetworkServer()
		if err != nil {
			fmt.Printf("UblockNetworkServer error: %s\n", err.Error())
		}
	}
}

// UblockNetworkReader gets Ublocks from the remote host and puts em in the
// channel.  It'll try to fill the channel buffer.
func UblockNetworkServer() error {

	listener, err := net.Listen("tcp", "127.0.0.1:8338")
	if err != nil {
		return err
	}
	con, err := listener.Accept()
	if err != nil {
		return err
	}

	go pushBlocks(con)

	defer listener.Close()

	return nil
}

func pushBlocks(c net.Conn) {
	var curHeight int32
	defer c.Close()
	err := binary.Read(c, binary.BigEndian, &curHeight)
	if err != nil {
		fmt.Printf("pushBlocks Read %s\n", err.Error())
		return
	}

	for ; ; curHeight++ {
		ud, err := util.GetUDataFromFile(curHeight)
		if err != nil {
			fmt.Printf("pushBlocks GetUDataFromFile %s\n", err.Error())
			return
		}

		blk, err := util.GetRawBlockFromFile(curHeight, util.OffsetFilePath)
		if err != nil {
			fmt.Printf("pushBlocks GetRawBlockFromFile %s\n", err.Error())
			return
		}

		// put proofs & block together, send that over
		ub := util.UBlock{ExtraData: ud, Block: blk}
		err = ub.Serialize(c)
		if err != nil {
			fmt.Printf("pushBlocks ub.Serialize %s\n", err.Error())
			return
		}
	}
}
