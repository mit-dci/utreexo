package bridgenode

import (
	"encoding/binary"
	"fmt"
	"net"

	"github.com/mit-dci/utreexo/util"
)

// blockServer listens on a TCP port for incoming connections, then gives
// ublocks blocks over that connection
func blockServer(endHeight int32, dataDir string, haltRequest, haltAccept chan bool) {

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

	cons := make(chan net.Conn)
	go acceptConnections(listener, cons)

	for {
		select {
		case <-haltRequest:
			listener.Close()
			haltAccept <- true
			close(cons)
			return
		case con := <-cons:
			go pushBlocks(con, endHeight, dataDir)
		}
	}
}

func acceptConnections(listener *net.TCPListener, cons chan net.Conn) {
	for {
		select {
		case <-cons:
			// cons got closed, stop accepting new connections
			return
		default:
		}

		con, err := listener.Accept()
		if err != nil {
			fmt.Printf("blockServer accept error: %s\n", err.Error())
			return
		}

		cons <- con
	}
}

func pushBlocks(c net.Conn, endHeight int32, blockDir string) {
	var curHeight int32
	defer c.Close()
	err := binary.Read(c, binary.BigEndian, &curHeight)
	if err != nil {
		fmt.Printf("pushBlocks Read %s\n", err.Error())
		return
	}
	fmt.Printf("start serving %s height %d\n", c.RemoteAddr().String(), curHeight)

	for ; curHeight < endHeight; curHeight++ {
		// fmt.Printf("push %d\n", curHeight)
		udb, err := util.GetUDataBytesFromFile(curHeight)
		if err != nil {
			fmt.Printf("pushBlocks GetUDataBytesFromFile %s\n", err.Error())
			return
		}
		// fmt.Printf("h %d read %d byte udb\n", curHeight, len(udb))

		blkbytes, err := GetBlockBytesFromFile(curHeight, util.OffsetFilePath, blockDir)
		if err != nil {
			fmt.Printf("pushBlocks GetRawBlockFromFile %s\n", err.Error())
			return
		}

		// first send the block bytes
		_, err = c.Write(blkbytes)
		if err != nil {
			fmt.Printf("pushBlocks blkbytes write %s\n", err.Error())
			return
		}

		// then send a 4 byte length, then udata
		// fmt.Printf("send ubb len %d\n", len(udb))
		err = binary.Write(c, binary.BigEndian, uint32(len(udb)))
		if err != nil {
			fmt.Printf("pushBlocks binary.Write %s\n", err.Error())
			return
		}
		_, err = c.Write(udb)
		if err != nil {
			fmt.Printf("pushBlocks ubb write %s\n", err.Error())
			return
		}
		// fmt.Printf("wrote %d bytes udb\n", n)
	}
	fmt.Printf("done pushing blocks to %s\n", c.RemoteAddr().String())
	c.Close()
}
