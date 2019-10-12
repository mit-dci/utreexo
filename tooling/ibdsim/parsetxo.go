package ibdsim

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mit-dci/utreexo/utreexo"
)

type TxoTTL struct {
	utreexo.Hash
	ExpiryBlock int32
}

// plusLine reads in a line of text, generates a utxo leaf, and determines
// if this is a leaf to remember or not.
func plusLine(s string) ([]utreexo.LeafTXO, error) {
	//	fmt.Printf("%s\n", s)
	parts := strings.Split(s[1:], ";")
	if len(parts) < 2 {
		return nil, fmt.Errorf("line %s has no ; in it", s)
	}
	txid := parts[0]
	postsemicolon := parts[1]

	indicatorHalves := strings.Split(postsemicolon, "x")
	ttldata := indicatorHalves[1]
	ttlascii := strings.Split(ttldata, ",")
	// the last one is always empty as there's a trailing ,
	ttlval := make([]int32, len(ttlascii)-1)
	for i, _ := range ttlval {
		if ttlascii[i] == "s" {
			//	ttlval[i] = 0
			// 0 means don't remember it! so 1 million blocks later
			ttlval[i] = 1 << 20
			continue
		}

		val, err := strconv.Atoi(ttlascii[i])
		if err != nil {
			return nil, err
		}
		ttlval[i] = int32(val)
	}

	txoIndicators := strings.Split(indicatorHalves[0], "z")

	numoutputs, err := strconv.Atoi(txoIndicators[0])
	if err != nil {
		return nil, err
	}
	if numoutputs != len(ttlval) {
		return nil, fmt.Errorf("%d outputs but %d ttl indicators",
			numoutputs, len(ttlval))
	}

	// numoutputs++ // for testnet3.txos

	unspend := make(map[int]bool)

	if len(txoIndicators) > 1 {
		unspendables := txoIndicators[1:]
		for _, zstring := range unspendables {
			n, err := strconv.Atoi(zstring)
			if err != nil {
				return nil, err
			}
			unspend[n] = true
		}
	}
	adds := []utreexo.LeafTXO{}
	for i := 0; i < numoutputs; i++ {
		if unspend[i] {
			continue
		}
		utxostring := fmt.Sprintf("%s;%d", txid, i)
		addData := utreexo.LeafTXO{
			Hash:     utreexo.HashFromString(utxostring),
			Duration: int32(ttlval[i])}
		//			Remember: lookahead >= ttlval[i]}
		adds = append(adds, addData)
		// fmt.Printf("expire in\t%d remember %v\n", ttlval[i], addData.Remember)
	}

	return adds, nil
}
