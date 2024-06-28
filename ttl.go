package main

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

type TTLCache struct {
	cache     []byte
	expiresAt time.Time
	ttl       time.Duration
	mutex     sync.Mutex
}

func NewTTLCache(defaultTTL time.Duration) *TTLCache {
	return &TTLCache{
		cache:     nil,
		expiresAt: time.Now(),
		ttl:       defaultTTL,
	}
}

func (c *TTLCache) Get() ([]byte, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.cache == nil || time.Now().After(c.expiresAt) {
		// Cache entry has expired, perform a GET request to refresh the value
		value, err := c.performGetRequest()
		if err != nil {
			return nil, err
		}

		// Update the cache entry with the new value and expiration time
		c.cache = value
		c.expiresAt = time.Now().Add(c.ttl)
	}

	return c.cache, nil
}

func (c *TTLCache) performGetRequest() ([]byte, error) {
	resp, err := http.Get("https://flakery.dev/api/deployments/lb-config-ng")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET request failed with status code %d", resp.StatusCode)
	}

	// Read the response body
	return io.ReadAll(resp.Body)

}

