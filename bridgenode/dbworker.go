package bridgenode

import (
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// DbWorker writes & reads/deletes everything to the db.
// It also generates TTLResultBlocks to send to the flat file worker
func DbWorker(
	dbWorkChan chan ttlRawBlock, ttlResultChan chan ttlResultBlock,
	lvdb *leveldb.DB, wg *sync.WaitGroup) {

	val := make([]byte, 4)

	for {
		dbBlock := <-dbWorkChan
		var batch leveldb.Batch
		// build the batch for writing to levelDB.
		// Just outpoints to index within block
		for i, op := range dbBlock.newTxos {
			binary.BigEndian.PutUint32(val, uint32(i))
			batch.Put(op[:], val)
		}
		// write all the new utxos in the batch to the DB
		err := lvdb.Write(&batch, nil)
		if err != nil {
			fmt.Println(err.Error())
		}
		batch.Reset()

		var trb ttlResultBlock

		trb.Height = dbBlock.blockHeight
		trb.Created = make([]txoStart, len(dbBlock.spentTxos))

		// now read from the DB all the spent txos and find their
		// position within their creation block
		for i, op := range dbBlock.spentTxos {
			batch.Delete(op[:]) // add this outpoint for deletion
			idxBytes, err := lvdb.Get(op[:], nil)
			if err != nil {
				fmt.Printf("can't find %x in db\n", op)
				panic(err)
			}

			// skip txos that live 0 blocks as they'll be deduped out of the
			// proofs anyway
			if dbBlock.spentStartHeights[i] != dbBlock.blockHeight {
				trb.Created[i].indexWithinBlock = binary.BigEndian.Uint32(idxBytes)
				trb.Created[i].createHeight = dbBlock.spentStartHeights[i]
			}
		}
		// send to flat ttl writer
		ttlResultChan <- trb
		err = lvdb.Write(&batch, nil) // actually delete everything
		if err != nil {
			fmt.Println(err.Error())
		}

		wg.Done()
	}
}

// Flags marking the status for the ttlIdx
type ttlIdxFlags uint8

const (
	// Is the utxo for this ttlIdx spent?
	spent ttlIdxFlags = 1 << iota

	// Is the utxo for this ttlIdx been modified?
	// The cached ttlIdx is different from the one on the disk
	modified

	// Is the utxo for this ttlIdx never been seen on disk?
	fresh
)

// The index and the status of this cached idx
type cachedTTLIdx struct {
	index uint32
	flags ttlIdxFlags
}

// Returns whether the cache is different from that on the disk
func (ttl *cachedTTLIdx) isModified() bool {
	return ttl.flags&modified == modified
}

// Returns whether the utxo that the cache represents has been spent
func (ttl *cachedTTLIdx) isSpent() bool {
	return ttl.flags&spent == spent
}

// Returns whether the cache has never been stored on disk
func (ttl *cachedTTLIdx) isFresh() bool {
	return ttl.flags&fresh == fresh
}

// Mark the cachedTTLIdx as modified
func (ttl *cachedTTLIdx) modify() {
	ttl.flags |= modified
}

// Clear the modified flag from the cachedTTLIdx
func (ttl *cachedTTLIdx) clearModify() {
	ttl.flags ^= modified
}

// Mark the cachedTTLIdx as spent
func (ttl *cachedTTLIdx) spend() {
	// no need to do anything if it's already marked as spent
	if ttl.isSpent() {
		return
	}
	ttl.flags |= spent | modified
}

// MemTTLdb is the in-memory cached ttldb
type MemTTLdb struct {
	// in-memory cache of the ttls
	cache map[[36]byte]*cachedTTLIdx

	// The memory usage in bytes that the memttldb is using
	memUsage int64

	// The maximum allowed memUsage for the memttldb
	flushMax int64

	// Whether to keep all the cachedTTLIdx in memory
	allInMem bool

	// the database itself on disk
	ttlDB *leveldb.DB

	// makes the Flush() wait for the in-process writes
	flushWait *sync.WaitGroup
}

// NewMemTTLdb returns an empty MemTTLdb
func NewMemTTLdb() *MemTTLdb {
	return &MemTTLdb{
		cache:    make(map[[36]byte]*cachedTTLIdx),
		flushMax: 4000000000, // 4GB
	}
}

// initMemDB initiatizes the membdb by buffering the ttdlb into
// a map in memory
func (mdb *MemTTLdb) InitMemDB(ttldbPath string, allInMem bool, lvdbOpt *opt.Options) error {
	// Open ttldb
	var err error
	mdb.ttlDB, err = leveldb.OpenFile(ttldbPath, lvdbOpt)
	if err != nil {
		return err
	}

	mdb.flushWait = &sync.WaitGroup{}
	mdb.allInMem = allInMem

	// Only fetch if we're gonna have the entire indexes in memory
	if mdb.allInMem {
		fmt.Println("Loading ttldb from disk...")
		var outpoint [36]byte
		iter := mdb.ttlDB.NewIterator(nil, nil)
		for iter.Next() {
			// key should be a serialized outpoint. Outpoints are 36 byte
			// 32 byte hash + 4 byte index within block
			if len(iter.Key()) != 36 {
				return fmt.Errorf("TTLDB corrupted."+
					"Outpoint should be 36 bytes but read %v\n",
					len(iter.Key()))
			}

			copy(outpoint[:], iter.Key()[:])
			mdb.cache[outpoint] = &cachedTTLIdx{
				index: binary.BigEndian.Uint32(iter.Value()),
			}
			mdb.memUsage += 36 + 4 + 1

		}

		iter.Release()
		err = iter.Error()
		if err != nil {
			return err
		}

		totalMiB := mdb.memUsage/(1024*1024) + 1
		fmt.Printf("Finished loading ttldb from disk. Using ~%v MiB\n", totalMiB)
	}
	return nil
}

// spendTTLEntry marks the passed in cachedIndex as spent in the MemTTLdb
func (mdb *MemTTLdb) spendTTLEntry(key [36]byte, addIfNil *cachedTTLIdx) error {
	entry := mdb.cache[key]

	// If we don't have an entry in cache and an entry was provided, we add it.
	if entry == nil && addIfNil != nil {
		mdb.Put(key, addIfNil)
		entry = addIfNil
	}

	// If it's nil or already spent, nothing to do.
	if entry == nil || entry.isSpent() {
		return nil
	}

	// If an entry is fresh, meaning that there hasn't been a flush since it was
	// introduced, it can simply be removed.
	if entry.isFresh() {
		// We don't delete it from the map, but set the value to nil, so that
		// later lookups for the entry know that the entry does not exist in the
		// database.
		mdb.cache[key] = nil
		return nil
	}

	// Mark the output as spent and modified.
	entry.flags |= spent | modified

	return nil
}

// Put puts a key-value in the cache. Does not alter the on disk ttldb.
// This function is not safe for concurrent access.
func (mdb *MemTTLdb) Put(key [36]byte, ttl *cachedTTLIdx) {
	if mdb.memUsage > mdb.flushMax {
		mdb.Flush()
	}

	mdb.flushWait.Add(1)
	defer mdb.flushWait.Done()
	mdb.cache[key] = ttl

	mdb.memUsage += 36 + 4 + 1
}

// Flush flushes the memory database to disk. This function is not safe for
// concurrent access
func (mdb *MemTTLdb) Flush() error {
	// Add one to round up the integer division.
	totalMiB := mdb.memUsage/(1024*1024) + 1
	fmt.Printf("Flushing UTXO cache of ~%v MiB to disk. For large sizes, "+
		"this can take up to several minutes...\n", totalMiB)

	mdb.flushWait.Wait()

	// Add since sigint will call the flush again. Make that flush wait
	// That one would save/delete whatever happend between this flush and
	// whenever the signal was given.
	mdb.flushWait.Add(1)
	defer mdb.flushWait.Done()

	val := make([]byte, 4)

	currentMemUsage := mdb.memUsage

	// Save the key-value pairs to the on-disk database
	for cacheKey, cacheValue := range mdb.cache {
		// if the cache is nil, it means it was already created and spent
		// simply remove from the cache
		if cacheValue == nil {
			mdb.memUsage -= (36 + 4 + 1)
			delete(mdb.cache, cacheKey)
			continue
		}

		// If the cache is not modified, then just remove
		// if all in mem, just continue
		if !cacheValue.isModified() {
			if !mdb.allInMem {
				mdb.memUsage -= (36 + 4 + 1)
				delete(mdb.cache, cacheKey)
			}

			continue
		}

		// If the cache is spent, then delete from the database on disk
		if cacheValue.isSpent() {
			mdb.ttlDB.Delete(cacheKey[:], nil)
			mdb.memUsage -= (36 + 4 + 1)
			delete(mdb.cache, cacheKey)
			continue
		}

		// only store if modified
		if cacheValue.isModified() {
			binary.BigEndian.PutUint32(val, uint32(cacheValue.index))
			err := mdb.ttlDB.Put(cacheKey[:], val, nil)
			if err != nil {
				return err
			}
			if !mdb.allInMem {
				mdb.memUsage -= (36 + 4 + 1)
				delete(mdb.cache, cacheKey)
			} else {
				// no longer modified since it was saved to disk
				cacheValue.clearModify()
			}
		}
	}

	// if all in mem and the mem usage after the flush + 10% of that is
	// greater than or equal to the mem usage before the flush
	if mdb.allInMem && currentMemUsage <= mdb.memUsage+(mdb.memUsage/10) {
		mdb.flushMax += (mdb.flushMax / 10)
	}

	fmt.Println("Flushed ttldb cache", mdb.memUsage, len(mdb.cache))
	return nil
}

// Close closes the memory database
func (mdb *MemTTLdb) Close() error {
	// Flush whatever we have left in the memory
	err := mdb.Flush()
	if err != nil {
		return err
	}

	return mdb.ttlDB.Close()
}

// Fetch the txoIndex from the database on disk
func (mdb *MemTTLdb) dbFetchSpentTxoIndex(fetch map[[36]byte]*cachedTTLIdx) error {
	for op, cached := range fetch {
		if cached == nil {
			idxBytes, err := mdb.ttlDB.Get(op[:], nil)
			if err != nil {
				return err
			}

			// Add the fetched index to the map
			fetch[op] = &cachedTTLIdx{
				index: binary.BigEndian.Uint32(idxBytes),
			}
		}
	}

	return nil
}

// MemDbWorker writes & reads/deletes everything to the memory cache and
// flushes the in-memory cache during shutdown.
// It also generates TTLResultBlocks to send to the flat file worker
func MemDbWorker(
	dbWorkChan chan ttlRawBlock, ttlResultChan chan ttlResultBlock,
	memTTLdb *MemTTLdb, wg *sync.WaitGroup) {

	for {
		dbBlock := <-dbWorkChan

		// Make temporary map
		fetchedIndexesDB := make(map[[36]byte]*cachedTTLIdx)

		// Grab the already cached indexes
		for _, op := range dbBlock.spentTxos {
			cached := memTTLdb.cache[op]
			if cached == nil {
				fetchedIndexesDB[op] = nil
				continue
			}
			fetchedIndexesDB[op] = cached
		}

		// WaitGroup for the database fetch
		var fetchwg sync.WaitGroup
		fetchwg.Add(1)

		// Fetch from the db asynchronously
		go func() {
			if !memTTLdb.allInMem {
				err := memTTLdb.dbFetchSpentTxoIndex(fetchedIndexesDB)
				if err != nil {
					panic(err)
				}
			}
			fetchwg.Done()
		}()

		// build the batch for writing to levelDB.
		// Just outpoints to index within block
		for i, op := range dbBlock.newTxos {
			ttl := &cachedTTLIdx{
				index: uint32(i),
				flags: fresh | modified,
			}
			memTTLdb.Put(op, ttl)
		}

		var trb ttlResultBlock

		trb.Height = dbBlock.blockHeight
		trb.Created = make([]txoStart, len(dbBlock.spentTxos))

		// wait for the indexes fetch from the db
		fetchwg.Wait()

		// now read from the DB all the spent txos and find their
		// position within their creation block
		for i, op := range dbBlock.spentTxos {
			cachedIndex := fetchedIndexesDB[op]
			idx := cachedIndex.index

			// skip txos that live 0 blocks as they'll be deduped out of the
			// proofs anyway
			if dbBlock.spentStartHeights[i] != dbBlock.blockHeight {
				trb.Created[i].indexWithinBlock = idx
				trb.Created[i].createHeight = dbBlock.spentStartHeights[i]
			}

			// Mark this cachedIndex as spent in the memttldb
			memTTLdb.spendTTLEntry(op, cachedIndex)
		}

		// send to flat ttl writer
		ttlResultChan <- trb

		wg.Done()
	}
}
