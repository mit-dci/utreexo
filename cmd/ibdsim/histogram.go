package ibdsim

import (
	"bufio"
	"fmt"
	"os"
)

func Histogram(ttlfn string, sig chan bool) error {

	go stopHistogram(sig)

	txofile, err := os.OpenFile(ttlfn, os.O_RDONLY, 0600)
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(txofile)
	scanner.Buffer(make([]byte, 1<<20), 1<<20) // 1MB should be enough?

	var hist [600000]int32 // big but doable
	var txos uint32
	for scanner.Scan() {
		if scanner.Text()[0] == '+' {
			adds, err := plusLine(scanner.Text())
			if err != nil {
				return err
			}
			for _, a := range adds {
				if a.Duration == 1<<20 {
					continue
				}
				hist[a.Duration]++
				txos++
			}
		}
	}
	fmt.Printf("total txos %d", txos)
	for i, amt := range hist {
		if amt != 0 {
			fmt.Printf("%d %d\n", i, amt)
		}
	}

	return scanner.Err()

}

func stopHistogram(sig chan bool) {
	<-sig
	fmt.Println("Exiting...")
	os.Exit(1)
}
