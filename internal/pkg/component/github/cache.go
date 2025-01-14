package github

import (
	"sync"
	"time"

	"github.com/konflux-ci/mintmaker/pkg/common"
)

type Cache struct {
	data sync.Map
}

func NewCache() *Cache {
	return &Cache{}
}

func (c *Cache) Get(key string) (interface{}, bool) {
	return c.data.Load(key)
}

func (c *Cache) Set(key string, value interface{}) {
	c.data.Store(key, value)
}

type TokenInfo struct {
	Token     string
	ExpiresAt time.Time
}

type TokenCache struct {
	mu      sync.RWMutex
	entries map[string]TokenInfo
}

func (c *TokenCache) Set(key string, tokenInfo TokenInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// initialize entries map in the first usage
	if c.entries == nil {
		c.entries = make(map[string]TokenInfo)
	}

	c.entries[key] = tokenInfo
}

func (c *TokenCache) Get(key string) (TokenInfo, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[key]
	if !exists {
		return TokenInfo{}, false
	}

	// when token is close to expiring, we can't use it
	if time.Until(entry.ExpiresAt) < common.GhTokenRenewThreshold {
		return TokenInfo{}, false
	}

	return entry, true
}
