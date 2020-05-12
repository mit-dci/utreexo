package bridgenode

import (
	"fmt"

	"github.com/mit-dci/utreexo/util"
)

// blockServer listens on a TCP port for incoming connections, then gives
// ublocks blocks over that connection

func blockServer(startHeight, endHeight int32) {
	for {
		fmt.Printf("starting UblockNetworkServer... ")
		err := util.UblockNetworkServer(startHeight, endHeight)
		if err != nil {
			fmt.Printf("UblockNetworkServer error: %s\n", err.Error())
		}
	}
}
