package cache

import (
	"sync"
	"time"
)

type entry struct {
	value     any
	expiresAt time.Time
}

type Cache struct {
	ttl   time.Duration
	mu    sync.RWMutex
	store map[string]entry
}

func New(ttl time.Duration) *Cache {
	return &Cache{
		ttl:   ttl,
		store: make(map[string]entry),
	}
}

func (c *Cache) Get(key string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, ok := c.store[key]
	if !ok || time.Now().After(item.expiresAt) {
		return nil, false
	}
	return item.value, true
}

func (c *Cache) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.store[key] = entry{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
}
