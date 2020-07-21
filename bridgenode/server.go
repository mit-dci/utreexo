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

	listenAdr, err := net.ResolveTCPAddr("tcp", "0.0.0.0:8338")
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
			go serveBlocksWorker(con, endHeight, dataDir)
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

// serveBlocksWorker gets height requests from client and sends out the ublock
// for that height
func serveBlocksWorker(c net.Conn, endHeight int32, blockDir string) {
	defer c.Close()
	fmt.Printf("start serving %s\n", c.RemoteAddr().String())
	var curHeight int32

	// first thing is push the tip height to the remote client so they know
	err := binary.Write(c, binary.BigEndian, endHeight)
	if err != nil {
		fmt.Printf("pushBlocks endHeight binary.Write %s\n", err.Error())
		return
	}

	for {
		err = binary.Read(c, binary.BigEndian, &curHeight)
		if err != nil {
			fmt.Printf("pushBlocks Read %s\n", err.Error())
			return
		}

		if curHeight > endHeight {
			break
		}

		// over the wire send:
		// 4 byte length prefix for the whole thing
		// then the block, then the udb len, then udb

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

		// first send 4 byte lenght for everything
		// fmt.Printf("h %d send len %d\n", curHeight, len(udb)+len(blkbytes))
		err = binary.Write(c, binary.BigEndian, uint32(len(udb)+len(blkbytes)))
		if err != nil {
			fmt.Printf("pushBlocks binary.Write %s\n", err.Error())
			return
		}
		// next, send the block bytes
		_, err = c.Write(blkbytes)
		if err != nil {
			fmt.Printf("pushBlocks blkbytes write %s\n", err.Error())
			return
		}
		// send 4 byte udata length
		// err = binary.Write(c, binary.BigEndian, uint32(len(udb)))
		// if err != nil {
		// 	fmt.Printf("pushBlocks binary.Write %s\n", err.Error())
		// 	return
		// }
		// last, send the udata bytes
		_, err = c.Write(udb)
		if err != nil {
			fmt.Printf("pushBlocks ubb write %s\n", err.Error())
			return
		}
		// fmt.Printf("wrote %d bytes udb\n", n)
	}
	fmt.Printf("hung up on %s\n", c.RemoteAddr().String())
}
