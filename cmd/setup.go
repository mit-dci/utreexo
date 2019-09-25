package main

import (
	log "github.com/ChainSafe/log15"
	"github.com/mit-dci/utreexo/config"
	"github.com/urfave/cli"
)

func makeNode(ctx *cli.Context) {
	dataDir := getDatabaseDir(ctx)
	log.Info(dataDir)
}

func getDatabaseDir(ctx *cli.Context) string {
	if file := ctx.GlobalString(DataDirFlag.Name); file != "" {
		return file
	} else {
		return config.DefaultDataDir()
	}
}