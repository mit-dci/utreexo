package main

import (
	"github.com/mit-dci/utreexo/tooling/blockparser"
	"github.com/mit-dci/utreexo/tooling/txottl"
	"os"

	log "github.com/ChainSafe/log15"
	"github.com/urfave/cli"
)

var (
	app       = cli.NewApp()
	coreFlags = []cli.Flag{
		ChainDbDirFlag,
	}
)

// init initializes CLI
func init() {
	app.Action = utreexo
	app.Name = "Utreexo"
	app.Usage = "Official Utreexo command-line interface"
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
func utreexo(ctx *cli.Context) {
	log.Info("Starting Utreexo...")

	// Necessary setup
	config := makeConfig(ctx)
	MakeDir()

	// Parse bitcoin state
	blockparser.Start(config)
	txottl.Start()

	node := makeNode(config)
	node.Start()
}
