package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime/pprof"
	"runtime/trace"
	"syscall"

	"github.com/mit-dci/utreexo/csn"
)

func main() {
	cfg, err := csn.Parse(os.Args[1:])
	if err != nil {
		fmt.Println(err)
		fmt.Println(csn.HelpMsg)
		os.Exit(1)
	}

	// listen for SIGINT, SIGTERM, or SIGQUIT from the os
	sig := make(chan bool, 1)
	handleIntSig(sig, cfg)

	err = csn.RunIBD(cfg, sig)
	if err != nil {
		panic(err)
	}
}

func handleIntSig(sig chan bool, cfg *csn.Config) {
	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)
	go func() {
		<-s
		if cfg.CpuProf != "" {
			pprof.StopCPUProfile()
		}
		if cfg.TraceProf != "" {
			trace.Stop()
		}
		sig <- true
	}()
}
