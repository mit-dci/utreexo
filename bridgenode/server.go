package bridgenode

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"

	"github.com/mit-dci/utreexo/util"
)

// blockServer listens on a TCP port for incoming connections, then gives
// ublocks blocks over that connection
func blockServer(dataDir string, haltRequest, haltAccept chan bool,
	newProofCondition *sync.Cond) {
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
			go pushBlocks(con, dataDir, newProofCondition)
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

func pushBlocks(c net.Conn, blockDir string, newProofCondition *sync.Cond) {
	var curHeight int32
	defer c.Close()
	err := binary.Read(c, binary.BigEndian, &curHeight)
	if err != nil {
		fmt.Printf("pushBlocks Read %s\n", err.Error())
		return
	}
	fmt.Printf("start serving %s height %d\n", c.RemoteAddr().String(), curHeight)

	var shouldWait bool
	for ; ; curHeight++ {
		// wait until the condition signals that new blocks+proof are ready to serve
		newProofCondition.L.Lock()
		if shouldWait {
			fmt.Println("pushBlocks: waiting for new proof")
			newProofCondition.Wait()
			shouldWait = false
			fmt.Println("pushBlocks: new proof found, ready to serve")
		}
		newProofCondition.L.Unlock()

		ud, err := util.GetUDataFromFile(curHeight)
		if err != nil {
			// TODO: only handle EOF instead of any error?
			fmt.Printf("pushBlocks GetUDataFromFile %s\n", err.Error())
			// try this height again next time
			curHeight--
			shouldWait = true
			continue
		}

		blk, _, err := GetRawBlockFromFile(curHeight, util.OffsetFilePath, blockDir)
		if err != nil {
			fmt.Printf("pushBlocks GetRawBlockFromFile %s\n", err.Error())
			// try this height again next time
			curHeight--
			shouldWait = true
			continue
		}

		// put proofs & block together, send that over
		ub := util.UBlock{ExtraData: ud, Block: blk}
		err = ub.Serialize(c)
		if err != nil {
			fmt.Printf("pushBlocks ub.Serialize %s\n", err.Error())
			return
		}
	}
	fmt.Printf("done pushing blocks to %s\n", c.RemoteAddr().String())
	c.Close()
}
