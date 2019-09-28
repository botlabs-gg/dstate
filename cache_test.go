package dstate

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newCache() *Cache {
	return NewCache(new(int64), new(int64))
}

func TestCacheSetGet(t *testing.T) {
	c := newCache()

	key := "123"
	c.Set(key, "hey")

	v := c.Get(key)
	if v == nil {
		t.Error("value did not set")
	}

	if v != "hey" {
		t.Error("value is not 'hey': ", v)
	}
}

func TestCacheEviction(t *testing.T) {
	c := newCache()

	key := "123"
	c.Set(key, "hey")

	n := c.EvictOldKeys(time.Now().Add(time.Hour))
	if n != 1 {
		t.Error("did not evict any keys")
		return
	}

	v := c.Get(key)
	if v != nil {
		t.Error("value is not nil after eviction")
	}
}

func TestCacheConcurrentSingleFetch(t *testing.T) {
	c := newCache()

	key := "123"

	didFetch := new(int64)

	var wg sync.WaitGroup

	fetchFunc := func() (interface{}, error) {
		t.Log("In fetch")

		if !atomic.CompareAndSwapInt64(didFetch, 0, 1) {
			t.Error("Concurrent fetch calls!")
			return 2, nil
		}

		defer wg.Done()

		time.Sleep(time.Second * 2)
		return 1, nil
	}

	wg.Add(1)

	go c.Fetch(key, fetchFunc)
	v, err := c.Fetch(key, fetchFunc)
	if err != nil {
		t.Log("Error fetching: ", err)
	}

	if v != 1 {
		t.Log("Incorrect value")
	}

	wg.Wait()
}
