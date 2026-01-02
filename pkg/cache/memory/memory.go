package memory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/veesix-networks/osvbng/pkg/cache"
)

type item struct {
	value      interface{}
	expiration time.Time
}

type Cache struct {
	items map[string]*item
	mu    sync.RWMutex
	stop  chan struct{}
}

func New() cache.Cache {
	c := &Cache{
		items: make(map[string]*item),
		stop:  make(chan struct{}),
	}

	go c.cleanup()

	return c
}

func (c *Cache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var expiration time.Time
	if ttl > 0 {
		expiration = time.Now().Add(ttl)
	}

	c.items[key] = &item{
		value:      value,
		expiration: expiration,
	}

	return nil
}

func (c *Cache) Get(ctx context.Context, key string) (interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, exists := c.items[key]
	if !exists {
		return nil, fmt.Errorf("key not found: %s", key)
	}

	if !item.expiration.IsZero() && time.Now().After(item.expiration) {
		return nil, fmt.Errorf("key expired: %s", key)
	}

	return item.value, nil
}

func (c *Cache) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.items, key)
	return nil
}

func (c *Cache) Scan(ctx context.Context, cursor uint64, pattern string, count int64) ([]string, uint64, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	keys := make([]string, 0)
	matched := 0
	skipped := 0

	for key := range c.items {
		if uint64(skipped) < cursor {
			skipped++
			continue
		}

		if matchPattern(pattern, key) {
			keys = append(keys, key)
			matched++
			if int64(matched) >= count {
				return keys, cursor + uint64(matched), nil
			}
		}
	}

	return keys, 0, nil
}

func matchPattern(pattern, key string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}

	i, j := 0, 0
	for i < len(pattern) && j < len(key) {
		if pattern[i] == '*' {
			if i == len(pattern)-1 {
				return true
			}
			for j < len(key) {
				if matchPattern(pattern[i+1:], key[j:]) {
					return true
				}
				j++
			}
			return false
		}
		if pattern[i] != key[j] {
			return false
		}
		i++
		j++
	}

	return i == len(pattern) && j == len(key)
}

func (c *Cache) Incr(ctx context.Context, key string) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	itm, exists := c.items[key]
	if !exists {
		c.items[key] = &item{value: int64(1)}
		return 1, nil
	}

	val, ok := itm.value.(int64)
	if !ok {
		return 0, fmt.Errorf("value is not an integer")
	}

	val++
	itm.value = val
	return val, nil
}

func (c *Cache) Decr(ctx context.Context, key string) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	itm, exists := c.items[key]
	if !exists {
		c.items[key] = &item{value: int64(-1)}
		return -1, nil
	}

	val, ok := itm.value.(int64)
	if !ok {
		return 0, fmt.Errorf("value is not an integer")
	}

	val--
	itm.value = val
	return val, nil
}

func (c *Cache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, exists := c.items[key]
	if !exists {
		return fmt.Errorf("key not found: %s", key)
	}

	if ttl > 0 {
		item.expiration = time.Now().Add(ttl)
	} else {
		item.expiration = time.Time{}
	}

	return nil
}

func (c *Cache) Close() error {
	close(c.stop)
	return nil
}

func (c *Cache) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.removeExpired()
		case <-c.stop:
			return
		}
	}
}

func (c *Cache) removeExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, item := range c.items {
		if !item.expiration.IsZero() && now.After(item.expiration) {
			delete(c.items, key)
		}
	}
}
