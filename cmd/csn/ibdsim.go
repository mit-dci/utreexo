package csn

import (
	"fmt"
	"os"

	"github.com/btcsuite/btcd/wire"
)

var maxmalloc uint64

func RunIBD(net wire.BitcoinNet, offsetfile string, ttldb string, sig chan bool) error {
	// start server & listen
	// go IBDServer()

	// start client & connect
	return IBDClient(net, offsetfile, ttldb, sig)
}

/*
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
*/

func stopRunIBD(sig chan bool, stopGoing chan bool, done chan bool) {
	<-sig
	fmt.Println("Exiting...")

	//Tell Runibd() to finish the block it's working on
	stopGoing <- true

	//Wait Runidb() says it's ok to quit
	<-done
	os.Exit(0)
}
