package dstate

import (
	"testing"
	"time"
)

func TestCacheSetGet(t *testing.T) {
	c := NewCache()

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
	c := NewCache()

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
