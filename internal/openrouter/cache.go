package openrouter

import (
	"sync"
	"time"
)

// Cache holds fetched provider+model data in memory with TTL.
type Cache struct {
	data      []ProviderModels
	fetchedAt time.Time
	err       error
	mu        sync.RWMutex
	ttl       time.Duration
}

// NewCache creates a cache with the given TTL.
func NewCache(ttl time.Duration) *Cache {
	return &Cache{ttl: ttl}
}

// Get returns cached data if fresh.
func (c *Cache) Get() ([]ProviderModels, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.data == nil || time.Since(c.fetchedAt) > c.ttl {
		return nil, false
	}
	return c.data, true
}

// Set stores fetched data.
func (c *Cache) Set(data []ProviderModels) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = data
	c.fetchedAt = time.Now()
	c.err = nil
}

// SetError stores an error from the last fetch attempt.
func (c *Cache) SetError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.err = err
}

// LastError returns the last fetch error, if any.
func (c *Cache) LastError() error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.err
}

// FetchOrGet fetches fresh data if cache is stale, otherwise returns cached data.
func (c *Cache) FetchOrGet(client *Client) ([]ProviderModels, error) {
	if data, ok := c.Get(); ok {
		return data, nil
	}
	data, err := client.FetchProviderModels()
	if err != nil {
		c.SetError(err)
		return nil, err
	}
	c.Set(data)
	return data, nil
}

// GlobalCache is a package-level cache shared across the application.
var GlobalCache = NewCache(5 * time.Minute)
