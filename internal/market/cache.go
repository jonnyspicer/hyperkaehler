package market

import (
	"sync"
	"time"

	"hyperkaehler/internal/strategy"
)

// Cache provides a TTL-based in-memory cache for market data.
type Cache struct {
	mu      sync.RWMutex
	markets map[string]cacheEntry
	ttl     time.Duration
}

type cacheEntry struct {
	data      strategy.MarketData
	fetchedAt time.Time
}

func NewCache(ttl time.Duration) *Cache {
	return &Cache{
		markets: make(map[string]cacheEntry),
		ttl:     ttl,
	}
}

func (c *Cache) Get(id string) (strategy.MarketData, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.markets[id]
	if !ok || time.Since(entry.fetchedAt) > c.ttl {
		return strategy.MarketData{}, false
	}
	return entry.data, true
}

func (c *Cache) Set(md strategy.MarketData) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.markets[md.ID] = cacheEntry{
		data:      md,
		fetchedAt: time.Now(),
	}
}

func (c *Cache) SetAll(markets []strategy.MarketData) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for _, md := range markets {
		c.markets[md.ID] = cacheEntry{
			data:      md,
			fetchedAt: now,
		}
	}
}

// All returns all non-expired entries.
func (c *Cache) All() []strategy.MarketData {
	c.mu.RLock()
	defer c.mu.RUnlock()

	now := time.Now()
	result := make([]strategy.MarketData, 0, len(c.markets))
	for _, entry := range c.markets {
		if now.Sub(entry.fetchedAt) <= c.ttl {
			result = append(result, entry.data)
		}
	}
	return result
}
