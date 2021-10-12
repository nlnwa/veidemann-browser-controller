package database

import (
	configV1 "github.com/nlnwa/veidemann-api/go/config/v1"
	"sync"
	"time"
)

type entry struct {
	expires time.Time
	configs []*configV1.ConfigObject
}

type cache struct {
	entries map[string]*entry
	ttl     time.Duration
	mu      sync.RWMutex
}

func newCache(ttl time.Duration) *cache {
	c := &cache{
		entries: make(map[string]*entry),
		ttl:     ttl,
	}
	go func() {
		for {
			c.purge()
			time.Sleep(ttl + 1*time.Minute)
		}
	}()
	return c
}

func (c *cache) purge() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, entry := range c.entries {
		if entry != nil && entry.expires.Before(time.Now()) {
			delete(c.entries, key)
		}
	}
}

func (c *cache) Set(key string, value *configV1.ConfigObject) {
	c.SetMany(key, []*configV1.ConfigObject{value})
}

func (c *cache) SetMany(key string, values []*configV1.ConfigObject) {
	if c.ttl == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = &entry{
		expires: time.Now().Add(c.ttl),
		configs: values,
	}
}

func (c *cache) Get(key string) *configV1.ConfigObject {
	configs := c.GetMany(key)
	if len(configs) > 0 {
		return configs[0]
	}
	return nil
}

func (c *cache) GetMany(key string) []*configV1.ConfigObject {
	if c.ttl == 0 {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if result, ok := c.entries[key]; ok {
		if result.expires.After(time.Now()) {
			return result.configs
		}
	}
	return nil
}
