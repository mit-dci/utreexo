package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime/pprof"
	"runtime/trace"
	"syscall"

	bridge "github.com/mit-dci/utreexo/bridgenode"
)

func main() {

	// parse the config
	cfg, err := bridge.Parse(os.Args[1:])
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// listen for SIGINT, SIGTERM, or SIGQUIT from the os
	sig := make(chan bool, 1)
	handleIntSig(sig, cfg)

	bridge.Start(*cfg, sig)
}

func handleIntSig(sig chan bool, cfg *bridge.Config) {
	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)
	go func() {
		<-s
		if cfg.CpuProf {
			pprof.StopCPUProfile()
		}

		if cfg.TraceProf {
			trace.Stop()
		}
		sig <- true
	}()
}
