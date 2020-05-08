package bridgenode

import "github.com/mit-dci/utreexo/util"

// blockServer listens on a TCP port for incoming connections, then gives
// ublocks blocks over that connection

func blockServer(startHeight, endHeight int32) {
	util.UblockNetworkServer(startHeight, endHeight)
}
