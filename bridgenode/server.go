package bridgenode

import (
	"encoding/binary"
	"fmt"
	"net"
	"path/filepath"

	"github.com/mit-dci/utreexo/util"
)

// blockServer listens on a TCP port for incoming connections, then gives
// ublocks blocks over that connection
func blockServer(endHeight int32, dataDir string, haltRequest, haltAccept chan bool) {
	blockDir := filepath.Join(dataDir, "/blocks/")

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
			go pushBlocks(con, blockDir)
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

func pushBlocks(c net.Conn, blockDir string) {
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

		blk, _, err := GetRawBlockFromFile(curHeight, util.OffsetFilePath, blockDir)
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
