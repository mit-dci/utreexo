package main

import (
	"github.com/urfave/cli"
)

var (
	// Bitcoin db location
	ChainDbDirFlag = cli.StringFlag{
		Name:  "chaindir",
		Usage: "Data directory for the Bitcoin chainstate",
	}
)
