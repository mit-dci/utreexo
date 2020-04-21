package util

type HashNpos struct {
	Result Hash
	Pos    uint64
}

func HashOne(l, r Hash, p uint64, hchan chan HashNpos) {
	var hnp HashNpos
	hnp.Pos = p
	hnp.Result = Parent(l, r)
	hchan <- hnp
}
