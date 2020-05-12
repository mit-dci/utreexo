package bridgenode

import (
	"encoding/binary"
	"fmt"
	"net"
	"time"

	"github.com/mit-dci/utreexo/util"
)

// blockServer listens on a TCP port for incoming connections, then gives
// ublocks blocks over that connection

func blockServer(endHeight int32, haltRequest, haltAccept chan bool) {

	listenAdr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:8338")
	if err != nil {
		fmt.Printf(err.Error())
		return
	}

	listener, err := net.ListenTCP("tcp", listenAdr)
	if err != nil {
		fmt.Printf(err.Error())
		return
	}

	for {
		var con net.Conn
		select {
		case <-haltRequest:
			listener.Close()
			haltAccept <- true
			return
		default:
		}
		// this... is how you do it?  Seems dumb to have an arbitrary timeout
		// here and not be able to somehow put the blocking listener.Accept()
		// itself in the select statement... huh.
		err = listener.SetDeadline(time.Now().Add(time.Second))
		if err != nil {
			fmt.Printf(err.Error())
			return
		}

		con, err = listener.Accept()
		if err != nil {
			if err.(*net.OpError).Timeout() {
				continue
			}
			fmt.Printf("blockServer accept error: %s\n", err.Error())
			continue
		}
		go pushBlocks(con)
	}
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
