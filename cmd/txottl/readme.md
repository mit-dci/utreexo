# txottl

txottl parses a text list of transactions and builds a database of how long txos last.  It can then append the txo durations to the transaction file.

This is very rough and only works well enough to get the database for utreexo IBD testing.  You have to compile with firstPass() and then comment it out and compile with secondPass() to get a different executable and run them sequentially.

The txo file that txottl wants can be generated from a branch of lit.  I'm sure there's a way to make one simpler and faster by parsing the blocks from a bitcoin's levelDB block store but I haven't done that.
