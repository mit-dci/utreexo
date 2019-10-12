package main

import (
	"github.com/mit-dci/utreexo/config"
	"github.com/mit-dci/utreexo/node"
	"github.com/urfave/cli"
	"os"
)

func MakeDir() {
	os.Mkdir(config.RootDbPath, os.ModePerm)
}

func makeNode(cfg *config.Config) *node.Node {
	node := &node.Node{}
	return node
}

func makeConfig(ctx *cli.Context) *config.Config {
	return &config.Config{
		ChainDir: override(ctx.GlobalString(ChainDbDirFlag.Name), config.DefaultConfig.ChainDir),
		TxoFilename: override("", config.DefaultConfig.TxoFilename),
		LevelDBPath: override("", config.DefaultConfig.LevelDBPath),
		MainnetTxo: override("", config.DefaultConfig.MainnetTxo),
	}
}

func override(a string, b string) string {
	if a != "" {
		return a
	} else {
		return b
	}
}