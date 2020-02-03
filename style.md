## Style guidelines for this repo

Mostly go has pretty strict guidlines about code should work, so stick to that.

I (adiabat) have vertical screens so I try to keep lines below 80ish characters.

I also try to keep go packages to under 20 files or so (utreexo/utreexo is getting close but has .txt, .md).
Try to keep files to under 1K lines, and to the extent possible I try to keep single functions / methods / structs to under 100 lines.  

If there are only a few of them, single letter variable names are fine; e.g. "self" in a method makes sense, "i" in a loop, etc.
Variable names in a struct though probably should be more descriptive.

Ints and uints should be avoided as they're architecture dependent; this stuff should compile and run just as well on a raspberry pi zero as well as a ryzen 7.  The compiler sometimes gives you ints (like from len()) but that can be cast to a defined length variable as soon as its obtained.

I like using big endian because then it's easier to read the hexidecimal.

Signed vs unsigned, I guess default to signed unless its for something that will be bit-shifted around a bunch.  btcsuite uses int32 for block heights so we can use that too.  In general use whatever types bitcoin does, like int64s for output amounts, even though they can never be negative.

Comments are good.  Code that was hard to write should be easy to read.  Hopefully.

Ask questions -- on IRC freenode in #utreexo 

