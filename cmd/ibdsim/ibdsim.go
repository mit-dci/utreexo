/* test the utreexo forest */

package ibdsim

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/mit-dci/utreexo/utreexo"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

var maxmalloc uint64

// run IBD from block proof data
// we get the new utxo info from the same txos text file
// the deletion data and proofs though, we get from the leveldb
// which was created by the bridge node.
func RunIBD(ttlfn string, schedFileName string, sig chan bool) error {

	go stopRunIBD(sig)
	txofile, err := os.OpenFile(ttlfn, os.O_RDONLY, 0600)
	if err != nil {
		return err
	}

	defer txofile.Close()

	// scheduleFile, err := os.OpenFile(*schedFileName, os.O_RDONLY, 0600)
	// if err != nil {
	// 	return err
	// }
	// defer scheduleFile.Close()

	proofDB, err := leveldb.OpenFile("./proofdb", &opt.Options{ReadOnly: true})
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(txofile)
	scanner.Buffer(make([]byte, 1<<20), 1<<20) // 1MB should be enough?

	// var scheduleBuffer []byte

	var height uint32
	height = 1

	var plustime time.Duration
	starttime := time.Now()

	totalTXOAdded := 0
	totalDels := 0

	var blockAdds []utreexo.LeafTXO
	var blockDels []utreexo.Hash

	var p utreexo.Pollard

	//	p.Minleaves = 1 << 30
	// p.Lookahead = 1000
	lookahead := int32(1000) // keep txos that last less than this many blocks

	fname := fmt.Sprintf("mem%s", schedFileName)

	for scanner.Scan() {
		// keep schedule buffer full, 100KB chunks at a time
		/* for clair schedule file
		if len(scheduleBuffer) < 100000 {
			nextBuf := make([]byte, 100000)
			_, err = scheduleFile.Read(nextBuf)
			if err != nil { // will error on EOF, deal w that
				return err
			}
			scheduleBuffer = append(scheduleBuffer, nextBuf...)
		}*/

		switch scanner.Text()[0] {
		case '-':
			// blarg, still need to read these for the dedupe part
			h := utreexo.HashFromString(scanner.Text()[1:])
			blockDels = append(blockDels, h)

		case '+':
			plusstart := time.Now()

			adds, err := plusLine(scanner.Text())
			if err != nil {
				return err
			}
			// read from the schedule to see if it's memorable
			for _, a := range adds {

				if a.Duration == 0 {
					continue
				}
				a.Remember = a.Duration < lookahead
				// scheduleBuffer[0]&(1<<(7-uint8(totalTXOAdded%8))) != 0

				totalTXOAdded++
				// if totalTXOAdded%8 == 0 {
				// after every 8 reads, pop the first byte off the front
				// scheduleBuffer = scheduleBuffer[1:]
				// }
				blockAdds = append(blockAdds, a)
			}

			donetime := time.Now()
			plustime += donetime.Sub(plusstart)

		case 'h':
			// dedupe, though in this case we don't care about dels,
			// just don't want to add stuff that shouldn't be there
			// utreexo.DedupeHashSlices(&blockAdds, &blockDels)

			// no longer need dedupe as 0-duration is filtered out before
			// right after plusLine

			// read a block proof from the db
			//TODO attach to normal block. Don't need leveldb
			bpBytes, err := proofDB.Get(utreexo.U32tB(height), nil)
			if err != nil {
				return err
			}

			bp, err := utreexo.FromBytesBlockProof(bpBytes)
			if err != nil {
				return err
			}
			if len(bp.Targets) > 0 {
				fmt.Printf("block proof for block %d targets: %v\n", height, bp.Targets)
			}
			err = p.IngestBlockProof(bp)
			if err != nil {
				return err
			}

			// totalTXOAdded += len(blockAdds)
			totalDels += len(bp.Targets)

			err = p.Modify(blockAdds, bp.Targets)
			if err != nil {
				return err
			}

			if height%100 == 0 {
				fmt.Printf("Block %d add %d del %d %s plus %.2f total %.2f \n",
					height, totalTXOAdded, totalDels, p.Stats(),
					plustime.Seconds(), time.Now().Sub(starttime).Seconds())
			}
			if height%1000 == 0 {
				fmt.Printf(MemStatString(fname))
			}

			blockAdds = []utreexo.LeafTXO{}
			blockDels = []utreexo.Hash{}
			height++
		default:
			panic("unknown string")
		}
	}

	err = proofDB.Close()
	if err != nil {
		return err
	}

	return scanner.Err()

	return nil
}

// build the bridge node / proofs
func BuildProofs(ttlfn string, sig chan bool) error {

	go stopBuildProofs(sig)

	fmt.Println(ttlfn)
	txofile, err := os.OpenFile(ttlfn, os.O_RDONLY, 0600)
	if err != nil {
		return err
	}

	proofDB, err := leveldb.OpenFile("./proofdb", nil)
	if err != nil {
		return err
	}

	f := utreexo.NewForest()

	scanner := bufio.NewScanner(txofile)

	scanner.Buffer(make([]byte, 1<<20), 1<<20) // 1MB should be enough?

	var height uint32
	var totalProofNodes int
	var plustime time.Duration
	starttime := time.Now()

	var blockAdds []utreexo.LeafTXO
	var blockDels []utreexo.Hash

	for scanner.Scan() {
		switch scanner.Text()[0] {
		case '-':
			h := utreexo.HashFromString(scanner.Text()[1:])
			//			fmt.Printf("%s -> %x\n", scanner.Text(), h)
			blockDels = append(blockDels, h)

		case '+':
			plusstart := time.Now()

			lineAdds, err := plusLine(scanner.Text())
			if err != nil {
				return err
			}

			blockAdds = append(blockAdds, lineAdds...)

			donetime := time.Now()
			plustime += donetime.Sub(plusstart)

		case 'h':

			utreexo.DedupeHashSlices(&blockAdds, &blockDels)

			height++

			// get set of currently known hashes here

			blockProof, err := f.ProveBlock(blockDels)
			if err != nil {
				return fmt.Errorf("block %d %s %s", height, f.Stats(), err.Error())
			}

			ok := f.VerifyBlockProof(blockProof)
			if !ok {
				return fmt.Errorf("VerifyBlockProof failed at block %d", height)
			}

			totalProofNodes += len(blockProof.Proof)
			//TODO dumb
			err = proofDB.Put(
				utreexo.U32tB(uint32(height)), blockProof.ToBytes(), nil)
			if err != nil {
				return err
			}

			//			for _, p := range proofs {
			//				ok := f.Verify(p)
			//				if !ok {
			//					return fmt.Errorf("proof position %p failed", p.Position)
			//				}
			//				fmt.Printf("proof %d: pos %d %d sibs %v\n",
			//					i, p.Position, len(p.Siblings), ok)

			//			}

			//			err := doReads(dels)
			//			if err != nil {
			//				return err
			//			}

			//			fmt.Printf("----------------------- call modify for block %d\n", height)

			_, err = f.Modify(blockAdds, blockProof.Targets)
			if err != nil {
				return err
			}

			// evict hashes from hashStash if they've been deleted
			//			for _, d := range dels {
			//				delete(hashStash, d)
			//			}

			blockAdds = []utreexo.LeafTXO{}
			blockDels = []utreexo.Hash{}
			//			fmt.Printf("done with block %d\n", height)

			if height%100 == 0 {
				fmt.Printf("Block %d %s plus %.2f total %.2f proofnodes %d \n",
					height, f.Stats(),
					plustime.Seconds(), time.Now().Sub(starttime).Seconds(),
					totalProofNodes)
			}

		default:
			panic("unknown string")
		}

	}
	err = proofDB.Close()
	if err != nil {
		return err
	}
	return scanner.Err()

}

func MemStatString(fname string) string {
	var s string
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	if m.Alloc > maxmalloc {
		maxmalloc = m.Alloc

		// overwrite profile to get max mem usage
		// (only measured at 1000 block increments...)
		memfile, err := os.Create(fname)
		if err != nil {
			panic(err.Error())
		}
		pprof.WriteHeapProfile(memfile)
		memfile.Close()
	}
	// For info on each, see: https://golang.org/pkg/runtime/#MemStats
	s = fmt.Sprintf("alloc %d MB max %d MB", m.Alloc>>20, maxmalloc>>20)
	s += fmt.Sprintf("\ttotalAlloc %d MB", m.TotalAlloc>>20)
	s += fmt.Sprintf("\tsys %d MB", m.Sys>>20)
	s += fmt.Sprintf("\tnumGC %d\n", m.NumGC)
	return s
}

func stopRunIBD(sig chan bool) {
	<-sig
	fmt.Println("Exiting...")
	os.Exit(1)
}


func stopBuildProofs(sig chan bool) {
	<-sig
	fmt.Println("Exiting...")
	os.Exit(1)
}
