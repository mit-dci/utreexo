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

	for {
		err := binary.Read(c, binary.BigEndian, &curHeight)
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
		// write the proof itself to the buffer
		_, err = buf.Write(udb)
		if err != nil {
			fmt.Printf("pushBlocks ubb write %s\n", err.Error())
			return
		}

		// Send to the client
		payload := buf.Bytes()
		// send the payload size to the client
		err = binary.Write(c, binary.BigEndian, uint32(len(payload)))
		if err != nil {
			fmt.Printf("pushBlocks len write %s\n", err.Error())
			return
		}
		// send the block + proofs to the client
		_, err = c.Write(payload)
		if err != nil {
			fmt.Printf("pushBlocks payload write %s\n", err.Error())
		}
		// fmt.Printf("wrote %d bytes udb\n", n)
	}
	fmt.Printf("hung up on %s\n", c.RemoteAddr().String())
}
