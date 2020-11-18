package csn

import "github.com/mit-dci/utreexo/accumulator"

// CsnHook is the main stateful struct for the Compact State Node.
// It keeps track of what block its on and what transactions it's looking for
type CompactState struct {
	pollard accumulator.Pollard
}
