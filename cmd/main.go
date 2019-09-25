package main

import (
	"os"

	log "github.com/ChainSafe/log15"
	"github.com/urfave/cli"
)

var (
	app       = cli.NewApp()
	coreFlags = []cli.Flag{
		DataDirFlag,
	}
)

// init initializes CLI
func init() {
	app.Action = utreexo
	app.Copyright = "Copyright 2019 ChainSafe Systems Authors"
	app.Name = "Utreexo"
	app.Usage = "Official Utreexo command-line interface"
	app.Author = "ChainSafe Systems 2019"
	app.Version = "0.0.1"
	app.Commands = []cli.Command{}
	app.Flags = append(app.Flags, coreFlags...)
}

func main() {
	if err := app.Run(os.Args); err != nil {
		log.Error("error starting app", "output", os.Stderr, "err", err)
		os.Exit(1)
	}
}

// utreexo is the main entrypoint into the utreexo system
func utreexo(ctx *cli.Context) error {
	log.Info("Starting Utreexo...")
	makeNode(ctx)
	return nil
}
