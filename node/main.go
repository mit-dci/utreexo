package node

import (
	log "github.com/ChainSafe/log15"
)

type Node struct {}

func (n *Node) Start() {
	log.Info("Starting node...")
}

func (n *Node) Stop() {
	log.Info("Stopping node...")
}