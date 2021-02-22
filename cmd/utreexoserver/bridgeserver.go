package main

import (
	"fmt"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"runtime/trace"
	"syscall"

	bridge "github.com/mit-dci/utreexo/bridgenode"
)

func main() {
	// The allocations from loading blocks from disk can cause
	// bursts of big memory allocations. This helps avoid that
	// by collecting garbage early.
	debug.SetGCPercent(20)

	// parse the config
	cfg, err := bridge.Parse(os.Args[1:])
	if err != nil {
		fmt.Println(err)
		fmt.Println(bridge.HelpMsg)
		os.Exit(1)
	}

	// listen for SIGINT, SIGTERM, or SIGQUIT from the os
	sig := make(chan bool, 1)
	handleIntSig(sig, cfg)

	err = bridge.Start(cfg, sig)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func handleIntSig(sig chan bool, cfg *bridge.Config) {
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

		if cfg.MemProf != "" {
			f, err := os.Create(cfg.MemProf)
			if err != nil {
				fmt.Println(err)
			}
			runtime.GC()
			pprof.WriteHeapProfile(f)

		}
		sig <- true
	}()
}
