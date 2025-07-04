// Copyright 2024 Red Hat, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package github

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/konflux-ci/mintmaker/internal/pkg/config"
)

// StaleAllowedCache refreshes data in the background while optionally
// serving stale data to maintain low latency during refreshes.
type StaleAllowedCache struct {
	data              sync.Map
	expiry            atomic.Value
	refreshMutex      sync.Mutex
	refreshInProgress bool
	refreshDone       chan struct{}
	refreshPeriod     time.Duration
	refreshFunc       func() (interface{}, error)
}

func NewStaleAllowedCache(refreshPeriod time.Duration, refreshFunc func() (interface{}, error)) *StaleAllowedCache {
	expiryTime := time.Now().Add(refreshPeriod)
	c := &StaleAllowedCache{
		refreshPeriod: refreshPeriod,
		refreshFunc:   refreshFunc,
	}
	c.expiry.Store(expiryTime)
	return c
}

// Get returns data from the cache, possibly stale data if a refresh
// is currently in progress. On first access, this call will block
// until data is available.
func (c *StaleAllowedCache) Get(key string) (interface{}, bool) {
	value, ok := c.data.Load(key)
	expiryTime := c.expiry.Load().(time.Time)

	needsRefresh := !ok || time.Now().After(expiryTime)
	if !needsRefresh {
		return value, ok
	}
	// Refresh the cache and get data
	return c.getWithRefresh(key)
}

func (c *StaleAllowedCache) getWithRefresh(key string) (interface{}, bool) {
	c.refreshMutex.Lock()

	// There is refresh in progress, wait for it to be finished and get the data
	if c.refreshInProgress {
		done := c.refreshDone
		c.refreshMutex.Unlock()

		<-done
		return c.data.Load(key)
	}

	// We're going to refresh the cache
	c.refreshInProgress = true
	c.refreshDone = make(chan struct{})
	refreshDone := c.refreshDone

	value, exists := c.data.Load(key)
	c.refreshMutex.Unlock()

	cleanup := func() {
		c.refreshMutex.Lock()
		c.refreshInProgress = false
		close(refreshDone) // Signal that refresh is complete
		c.refreshMutex.Unlock()
	}

	// We have existing data, return it and refresh in background
	if exists {
		// Refresh in background
		go func() {
			defer cleanup()
			// Call the refresh function
			if newData, err := c.refreshFunc(); err == nil {
				c.data.Store(key, newData)
				c.expiry.Store(time.Now().Add(c.refreshPeriod))
			}
		}()
		// Return current data (which might be old)
		return value, true
	}

	// No existing data, perform refresh synchronously
	defer cleanup()

	// Call the refresh function
	newData, err := c.refreshFunc()
	if err != nil {
		return nil, false
	}
	// Store the new data and return it
	c.data.Store(key, newData)
	c.expiry.Store(time.Now().Add(c.refreshPeriod))
	return newData, true
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

	c.entries[key] = tokenInfo
}

func (c *TokenCache) Get(key string) (TokenInfo, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[key]
	if !exists {
		return TokenInfo{}, false
	}
	cfg := config.GetConfig().GlobalConfig
	// when token is close to expiring, we can't use it
	if time.Until(entry.ExpiresAt) < cfg.GhTokenRenewThreshold {
		return TokenInfo{}, false
	}

	return entry, true
}
