package bridgenode

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"runtime/pprof"
	"runtime/trace"
	"time"

	"github.com/mit-dci/utreexo/util"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

func Start(cfg *Config, sig chan bool) error {
	if cfg.CpuProf != "" {
		f, err := os.Create(cfg.CpuProf)
		if err != nil {
			return err
		}
		pprof.StartCPUProfile(f)
	}
	if cfg.MemProf != "" {
		f, err := os.Create(cfg.MemProf)
		if err != nil {
			return err
		}
		pprof.WriteHeapProfile(f)
	}
	if cfg.TraceProf != "" {
		f, err := os.Create(cfg.TraceProf)
		if err != nil {
			return err
		}
		trace.Start(f)
	}

	// If serve option wasn't given
	if !cfg.serve {
		err := BuildProofs(cfg, sig)
		if err != nil {
			return errBuildProofs(err)
		}
	}

	if !cfg.noServe {
		// serve when finished
		err := ArchiveServer(cfg, sig)
		if err != nil {
			return errArchiveServer(err)
		}
	}

	return nil
}

func ArchiveServer(cfg *Config, sig chan bool) error {
	// Channel to alert the tell the main loop it's ok to exit
	haltRequest := make(chan bool, 1)

	// Channel for ServeBlock() to wait
	haltAccept := make(chan bool, 1)

	// Handle user interruptions
	go stopServer(sig, haltRequest, haltAccept)

	if !util.HasAccess(cfg.BlockDir) {
		return errNoDataDir(cfg.BlockDir)
	}

	// TODO ****** server shouldn't need levelDB access, fix this
	// Open leveldb
	o := opt.Options{
		CompactionTableSizeMultiplier: 8,
		Compression:                   opt.NoCompression,
	}
	lvdb, err := leveldb.OpenFile(cfg.UtreeDir.Ttldb, &o)
	if err != nil {
		fmt.Printf("initialization error.  If your .blk and .dat files are ")
		fmt.Printf("not in %s, specify alternate path with -datadir\n.", cfg.BlockDir)
		return err
	}
	defer lvdb.Close()
	// **********************************

	// Init forest and variables. Resumes if the data directory exists
	maxHeight, err := restoreHeight(cfg)
	if err != nil {
		return err
	}

	blockServer(maxHeight, cfg, haltRequest, haltAccept)
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
	endHeight int32, cfg *Config, haltRequest, haltAccept chan bool) {

	// before doing anything... this breaks
	/*
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
	*/
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
			go serveBlocksWorker(cfg.UtreeDir, con, endHeight, cfg.BlockDir)
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
func serveBlocksWorker(UtreeDir utreeDir,
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
		if direction == 1 && curHeight > toHeight {
			// forwards request of height above toHeight
			break
		} else if direction == -1 && curHeight < toHeight {
			// backwards request of height below toHeight
			break
		}

		udb, err := GetUDataBytesFromFile(UtreeDir.ProofDir, curHeight)
		if err != nil {
			fmt.Printf("pushBlocks GetUDataBytesFromFile %s\n", err.Error())
			break
		}

		blkbytes, err := GetBlockBytesFromFile(
			curHeight, UtreeDir.OffsetDir.OffsetFile, blockDir)
		if err != nil {
			fmt.Printf("pushBlocks GetRawBlockFromFile %s\n", err.Error())
			break
		}

		// send
		_, err = c.Write(append(blkbytes, udb...))
		if err != nil {
			fmt.Printf("pushBlocks blkbytes write %s\n", err.Error())
			break
		}
	}
	err = c.Close()
	if err != nil {
		fmt.Print(err.Error())
	}
	fmt.Printf("hung up on %s\n", c.RemoteAddr().String())
}

// GetUDataBytesFromFile reads the proof data from proof.dat and proofoffset.dat
// and gives the proof & utxo data back.
// Don't ask for block 0, there is no proof for that.
// But there is an offset for block 0, which is 0, so it collides with block 1
func GetUDataBytesFromFile(proofDir proofDir, height int32) (b []byte, err error) {
	if height == 0 {
		err = fmt.Errorf("Block 0 is not in blk files or utxo set")
		return
	}

	var offset int64
	var size uint32
	var readMagic [4]byte
	realMagic := [4]byte{0xaa, 0xff, 0xaa, 0xff}
	offsetFile, err := os.OpenFile(proofDir.pOffsetFile, os.O_RDONLY, 0600)
	if err != nil {
		return
	}

	proofFile, err := os.OpenFile(proofDir.pFile, os.O_RDONLY, 0600)
	if err != nil {
		return
	}

	// offset file consists of 8 bytes per block
	// tipnum * 8 gives us the correct position for that block
	// Note it's currently a int64, can go down to int32 for split files
	_, err = offsetFile.Seek(int64(8*height), 0)
	if err != nil {
		err = fmt.Errorf("offsetFile.Seek %s", err.Error())
		return
	}

	// read the offset of the block we want from the offset file
	err = binary.Read(offsetFile, binary.BigEndian, &offset)
	if err != nil {
		err = fmt.Errorf("binary.Read h %d offset %d %s", height, offset, err.Error())
		return
	}

	// seek to that offset
	_, err = proofFile.Seek(offset, 0)
	if err != nil {
		err = fmt.Errorf("proofFile.Seek %s", err.Error())
		return
	}

	// first read 4-byte magic aaffaaff
	n, err := proofFile.Read(readMagic[:])
	if err != nil {
		return nil, err
	}
	if n != 4 {
		return nil, fmt.Errorf("only read %d bytes from proof file", n)
	}
	if readMagic != realMagic {
		return nil, fmt.Errorf("expect magic %x but read %x h %d offset %d",
			realMagic, readMagic, height, offset)
	}

	// fmt.Printf("height %d offset %d says size %d\n", height, offset, size)

	err = binary.Read(proofFile, binary.BigEndian, &size)
	if err != nil {
		return
	}

	if size > 1<<24 {
		return nil, fmt.Errorf(
			"size at offest %d says %d which is too big", offset, size)
	}
	// fmt.Printf("GetUDataBytesFromFile read size %d ", size)
	b = make([]byte, size)

	_, err = proofFile.Read(b)
	if err != nil {
		err = fmt.Errorf("proofFile.Read(ubytes) %s", err.Error())
		return
	}

	err = offsetFile.Close()
	if err != nil {
		return
	}
	err = proofFile.Close()
	if err != nil {
		return
	}
	return
}
