package main

import (
	"github.com/chainsafe/utreexo/config"
	"github.com/urfave/cli"
)

var (
	// Bitcoin db location
	DataDirFlag = cli.StringFlag{
		Name:  "datadir",
		Usage: "Data directory for the database",
		Value: config.DefaultDataDir(),
	}
)
