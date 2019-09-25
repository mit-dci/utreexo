package main

import (
	"github.com/chainsafe/utreexo/config"
	"github.com/chainsafe/utreexo/node"
	"github.com/urfave/cli"
)

func makeNode(cfg *config.Config) *node.Node {
	node := &node.Node{}
	return node
}

func makeConfig(ctx *cli.Context) *config.Config {
	return &config.Config{
		DataDir: getDatabaseDir(ctx),
	}
}

func getDatabaseDir(ctx *cli.Context) string {
	if file := ctx.GlobalString(DataDirFlag.Name); file != "" {
		return file
	} else {
		return config.DefaultDataDir()
	}
}
