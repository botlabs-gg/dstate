package dstate

import (
	"sync/atomic"
	"time"
)

type Cache struct {
	store     map[interface{}]*Bucket
	cacheHits *int64
	cacheMiss *int64
}

func NewCache(hit, miss *int64) *Cache {
	return &Cache{
		store:     make(map[interface{}]*Bucket),
		cacheHits: hit,
		cacheMiss: miss,
	}
}

type Bucket struct {
	Value   interface{}
	Created time.Time
}

// Get retrieves an item from the cache, this does not mutate anything and is safe to use with a read lock
func (c *Cache) Get(key interface{}) interface{} {
	bucket := c.store[key]
	if bucket == nil {
		return nil
	}

	return bucket.Value
}

// Set stores an item in the cache, this does mutate and needs to be used with a full lock
func (c *Cache) Set(key, value interface{}) {
	bucket := c.store[key]
	if bucket == nil {
		bucket = &Bucket{
			Created: time.Now(),
		}
		c.store[key] = bucket
	}

	bucket.Value = value
}

// Del deletes an item from the cache, this mutates state and needs to be used with a full lock, if its used from multiple goroutines
func (c *Cache) Del(key interface{}) {
	delete(c.store, key)
}

type CacheFetchFunc func() (value interface{}, err error)

// Fetch either retrieves an item directly from the cache if available, or calls the passed fetchFunc to then set it in cache and return it
func (c *Cache) Fetch(key interface{}, fetchFunc CacheFetchFunc) (interface{}, error) {
	v := c.Get(key)
	if v != nil {
		atomic.AddInt64(c.cacheHits, 1)
		return v, nil
	}

	atomic.AddInt64(c.cacheMiss, 1)
	value, err := fetchFunc()
	if err != nil {
		return value, err
	}

	c.Set(key, value)

	return value, nil
}

// EvictOldKeys evicts old keys created before treshold
func (c *Cache) EvictOldKeys(treshold time.Time) (evicted int) {
	for k, v := range c.store {
		if v.Created.Before(treshold) {
			delete(c.store, k)
			evicted++
		}
	}

	return
}
