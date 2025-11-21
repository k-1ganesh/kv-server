package cache

import (
	"container/list"
	"sync"
)

const SHARD_COUNT = 32

type entry struct {
	key   string
	value string
}

type lruShard struct {
	capacity int
	cache    map[string]*list.Element
	lru      *list.List
	mu       sync.Mutex 
	hits     uint64
	misses   uint64
}

// ShardedCache is the wrapper that manages the 8 internal shards.
type ShardedCache struct {
	shards [SHARD_COUNT]*lruShard
}

// NewShardedCache creates 8 distinct LRU caches, dividing capacity among them.
func NewShardedCache(totalCapacity int) *ShardedCache {
	sc := &ShardedCache{}

	
	shardCap := totalCapacity / SHARD_COUNT
	if shardCap < 1 {
		shardCap = 1
	}

	// Initialize each shard
	for i := 0; i < SHARD_COUNT; i++ {
		sc.shards[i] = &lruShard{
			capacity: shardCap,
			cache:    make(map[string]*list.Element),
			lru:      list.New(),
		}
	}

	return sc
}


func hash(key string) uint64 {
	var h uint64 = 14695981039346656037
	for i := 0; i < len(key); i++ {
		h ^= uint64(key[i])
		h *= 1099511628211
	}
	return h
}

// getShard determines which shard owns the key
func (sc *ShardedCache) getShard(key string) *lruShard {
	h := hash(key)
	// Fast bitwise modulo: h % 8 == h & 7
	return sc.shards[h&(SHARD_COUNT-1)]
}

// --- Public API ---

func (sc *ShardedCache) Get(key string) (string, bool) {
	shard := sc.getShard(key)

	shard.mu.Lock()
	defer shard.mu.Unlock()

	if elem, ok := shard.cache[key]; ok {
		shard.lru.MoveToFront(elem)
		shard.hits++
		return elem.Value.(*entry).value, true
	}
	shard.misses++
	return "", false
}

func (sc *ShardedCache) Put(key, value string) {
	shard := sc.getShard(key)

	shard.mu.Lock()
	defer shard.mu.Unlock()

	// Check for update
	if elem, ok := shard.cache[key]; ok {
		shard.lru.MoveToFront(elem)
		elem.Value.(*entry).value = value
		return
	}

	// Check for eviction
	if shard.lru.Len() >= shard.capacity {
		oldest := shard.lru.Back()
		if oldest != nil {
			shard.lru.Remove(oldest)
			delete(shard.cache, oldest.Value.(*entry).key)
		}
	}

	// Add new
	elem := shard.lru.PushFront(&entry{key: key, value: value})
	shard.cache[key] = elem
}

func (sc *ShardedCache) Delete(key string) {
	shard := sc.getShard(key)

	shard.mu.Lock()
	defer shard.mu.Unlock()

	if elem, ok := shard.cache[key]; ok {
		shard.lru.Remove(elem)
		delete(shard.cache, key)
	}
}

func (sc *ShardedCache) GetStats() (totalHits, totalMisses uint64) {
	// Aggregate stats from all shards
	for _, shard := range sc.shards {
		shard.mu.Lock()
		totalHits += shard.hits
		totalMisses += shard.misses
		shard.mu.Unlock()
	}
	return
}
