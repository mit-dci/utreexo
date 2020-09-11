package bridgenode

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/mit-dci/utreexo/util"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
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

	// TODO ****** server shouldn't need levelDB access, fix this
	ttlpath := "utree/" + param.Name + "ttldb"
	// Open leveldb
	o := opt.Options{CompactionTableSizeMultiplier: 8}
	lvdb, err := leveldb.OpenFile(ttlpath, &o)
	if err != nil {
		fmt.Printf("initialization error.  If your .blk and .dat files are ")
		fmt.Printf("not in %s, specify alternate path with -datadir\n.", dataDir)
		return err
	}
	defer lvdb.Close()
	// **********************************

	// Init forest and variables. Resumes if the data directory exists
	maxHeight, err := restoreHeight()
	if err != nil {
		return err
	}

	blockServer(maxHeight, dataDir, haltRequest, haltAccept, lvdb)

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
func blockServer(endHeight int32, dataDir string, haltRequest,
	haltAccept chan bool, lvdb *leveldb.DB) {
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
			go serveBlocksWorker(con, endHeight, dataDir, lvdb)
		}
	}
}

func acceptConnections(listener *net.TCPListener, cons chan net.Conn) {
	fmt.Printf("listening for connections on %s\n", listener.Addr().String())
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
func serveBlocksWorker(
	c net.Conn, endHeight int32, blockDir string, lvdb *leveldb.DB) {
	defer c.Close()
	fmt.Printf("start serving %s\n", c.RemoteAddr().String())
	var fromHeight, toHeight int32
	for {
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
			break
		}

		for curHeight := fromHeight; ; curHeight += direction {
			if direction == 1 && curHeight > toHeight {
				// forwards request of height above toHeight
				break
			} else if direction == -1 && curHeight < toHeight {
				// backwards request of height below toHeight
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
			ud, err := util.UDataFromBytes(udb)
			if err != nil {
				fmt.Printf("pushBlocks UDataFromBytes %s\n", err.Error())
				return
			}

			// set leaf ttl values
			// now we get them all for free

			/*
				ud.LeafTTLs = make([]int32, len(ud.UtxoData))
				for i, utxo := range ud.UtxoData {
					heightBytes, err := lvdb.Get(
						[]byte(util.OutpointToBytes(&utxo.Outpoint)), nil)
					if err != nil {
						if err == leveldb.ErrNotFound {
							// outpoint not spend yet, set to a billion for unknown
							ud.LeafTTLs[i] = 1 << 30
							continue
						}
						panic(err)
					}
					ud.LeafTTLs[i] = int32(binary.BigEndian.Uint32(heightBytes))
				}
			*/
			udb = ud.ToBytes()

			// fmt.Printf("h %d read %d byte udb\n", curHeight, len(udb))
			blkbytes, err := GetBlockBytesFromFile(
				curHeight, util.OffsetFilePath, blockDir)
			if err != nil {
				fmt.Printf("pushBlocks GetRawBlockFromFile %s\n", err.Error())
				return
			}

			// first send 4 byte length for everything
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

	}
	fmt.Printf("hung up on %s\n", c.RemoteAddr().String())
}
