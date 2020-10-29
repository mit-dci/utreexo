package bridgenode

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/mit-dci/utreexo/util"
)

func ArchiveServer(param chaincfg.Params, dataDir string, sig chan bool) error {

	// Channel to alert the tell the main loop it's ok to exit
	haltRequest := make(chan bool, 1)

	// Channel for ServeBlock() to wait
	haltAccept := make(chan bool, 1)

	// Handle user interruptions
	go stopServer(sig, haltRequest, haltAccept)

	_, err := os.Stat(dataDir)
	if os.IsNotExist(err) {
		return fmt.Errorf("%s not found, can't serve blocks\n", dataDir)
	}

	// Init forest and variables. Resumes if the data directory exists
	maxHeight, err := restoreHeight()
	if err != nil {
		return err
	}

	blockServer(maxHeight, dataDir, haltRequest, haltAccept)

	return nil
}

// stopServer listens for the signal from the OS and initiates an exit sequence
func stopServer(sig, haltRequest, haltAccept chan bool) {

	// Listen for SIGINT, SIGQUIT, SIGTERM
	<-sig
	haltRequest <- true
	// Sometimes there are bugs that make the program run forever.
	// Utreexo binary should never take more than 10 seconds to exit
	go func() {
		time.Sleep(2 * time.Second)
		fmt.Println("Exit timed out. Force quitting.")
		os.Exit(1)
	}()

	// Tell the user that the sig is received
	fmt.Println("User exit signal received. Exiting...")

	// Wait until server says it's ok to exit
	<-haltAccept
	os.Exit(0)
}

// blockServer listens on a TCP port for incoming connections, then gives
// ublocks blocks over that connection
func blockServer(
	endHeight int32, dataDir string, haltRequest, haltAccept chan bool) {

	// before doing anything... this breaks

	udb, err := util.GetUDataBytesFromFile(385)
	if err != nil {
		fmt.Printf(err.Error())
		panic("ded")
	}

	var buf bytes.Buffer
	var ud util.UData
	buf.Write(udb)
	fmt.Printf("buf len %d\n", buf.Len())
	err = ud.Deserialize(&buf)
	if err != nil {
		fmt.Printf(" ubd %s\n", err.Error())
		panic("ded")
	}
	fmt.Printf(ud.AccProof.ToString())
	fmt.Printf("h %d ud %d targets %d ttls\n",
		ud.Height, len(ud.AccProof.Targets), len(ud.TxoTTLs))

	// --------------

	fmt.Printf("serving up to & including block height %d\n", endHeight)
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
	fmt.Printf("listening for connections on %s\n", listener.Addr().String())
	for {
		select {
		case <-cons:
			// cons got closed, stop accepting new connections
			fmt.Printf("dropped con\n")
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
func serveBlocksWorker(
	c net.Conn, endHeight int32, blockDir string) {
	defer c.Close()
	fmt.Printf("start serving %s\n", c.RemoteAddr().String())
	var fromHeight, toHeight int32

	err := binary.Read(c, binary.BigEndian, &fromHeight)
	if err != nil {
		fmt.Printf("pushBlocks Read %s\n", err.Error())
		return
	}

	err = binary.Read(c, binary.BigEndian, &toHeight)
	if err != nil {
		fmt.Printf("pushBlocks Read %s\n", err.Error())
		return
	}

	var direction int32 = 1
	if toHeight < fromHeight {
		// backwards
		direction = -1
	}

	if toHeight > endHeight {
		toHeight = endHeight
	}

	if fromHeight > endHeight {
		fmt.Printf("%s wanted %d but have %d\n",
			c.LocalAddr().String(), fromHeight, endHeight)
		return
	}

	for curHeight := fromHeight; ; curHeight += direction {
		// fmt.Printf("client %s, curHeight %d\t", c.RemoteAddr().String(), curHeight)
		if direction == 1 && curHeight > toHeight {
			// forwards request of height above toHeight
			break
		} else if direction == -1 && curHeight < toHeight {
			// backwards request of height below toHeight
			break
		}

		udb, err := util.GetUDataBytesFromFile(curHeight)
		if err != nil {
			fmt.Printf("pushBlocks GetUDataBytesFromFile %s\n", err.Error())
			break
		}

		blkbytes, err := GetBlockBytesFromFile(
			curHeight, util.OffsetFilePath, blockDir)
		if err != nil {
			fmt.Printf("pushBlocks GetRawBlockFromFile %s\n", err.Error())
			break
		}

		// TODO this is just for testing -- we don't have to deserialize
		// the whole thing then re-serialize to send it
		// Also currently skipping the size prefix to run deserialize
		// ... just like bitcoin blocks

		var buf bytes.Buffer
		var ub util.UBlock
		buf.Write(blkbytes)
		buf.Write(udb)

		// fmt.Printf("buf len %d\n", buf.Len())
		buflen := buf.Len()
		// should be able to read the whole thing from the buffer
		err = ub.Deserialize(&buf)
		if err != nil {
			fmt.Printf("h %d ub.Deserialize %s\n", curHeight, err.Error())
			break
		}
		if buflen != ub.SerializeSize() {
			fmt.Printf("h %d buflen %d but sersize %d\n",
				curHeight, buflen, ub.SerializeSize())
			for i, stxo := range ub.UtreexoData.Stxos {
				fmt.Printf("%d %s\n", i, stxo.ToString())
			}
			for i, ttl := range ub.UtreexoData.TxoTTLs {
				fmt.Printf("%d %d\n", i, ttl)
			}
			fmt.Printf(ub.UtreexoData.AccProof.ToString())
		}

		// send
		_, err = c.Write(append(blkbytes, udb...))
		if err != nil {
			fmt.Printf("pushBlocks blkbytes write %s\n", err.Error())
			break
		}
		// fmt.Printf("sent %d bytes: %d block, %d udata\n",
		// n, len(blkbytes), len(udb))
		// fmt.Printf("udata hex: %x\n", udb)
	}
	err = c.Close()
	if err != nil {
		fmt.Print(err.Error())
	}
	fmt.Printf("hung up on %s\n", c.RemoteAddr().String())
}
