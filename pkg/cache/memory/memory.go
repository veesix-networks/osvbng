package memory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/veesix-networks/osvbng/pkg/cache"
)

type item struct {
	value      []byte
	expiration time.Time
}

type Cache struct {
	items sync.Map
	stop  chan struct{}
}

func New() cache.Cache {
	c := &Cache{
		stop: make(chan struct{}),
	}

	go c.cleanup()

	return c
}

func (c *Cache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	var expiration time.Time
	if ttl > 0 {
		expiration = time.Now().Add(ttl)
	}

	c.items.Store(key, &item{
		value:      value,
		expiration: expiration,
	})

	return nil
}

func (c *Cache) Get(ctx context.Context, key string) ([]byte, error) {
	v, exists := c.items.Load(key)
	if !exists {
		return nil, fmt.Errorf("key not found: %s", key)
	}

	itm := v.(*item)
	if !itm.expiration.IsZero() && time.Now().After(itm.expiration) {
		return nil, fmt.Errorf("key expired: %s", key)
	}

	return itm.value, nil
}

func (c *Cache) GetAll(ctx context.Context, pattern string) (map[string][]byte, error) {
	result := make(map[string][]byte)
	now := time.Now()

	c.items.Range(func(k, v any) bool {
		key := k.(string)
		itm := v.(*item)

		if !matchPattern(pattern, key) {
			return true
		}

		if !itm.expiration.IsZero() && now.After(itm.expiration) {
			return true
		}

		result[key] = itm.value
		return true
	})

	return result, nil
}

func (c *Cache) Delete(ctx context.Context, key string) error {
	c.items.Delete(key)
	return nil
}

func (c *Cache) Scan(ctx context.Context, cursor uint64, pattern string, count int64) ([]string, uint64, error) {
	keys := make([]string, 0)
	c.items.Range(func(k, _ any) bool {
		key := k.(string)
		if matchPattern(pattern, key) {
			keys = append(keys, key)
		}
		return true
	})

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
	for {
		v, loaded := c.items.Load(key)
		if !loaded {
			newItem := &item{value: []byte("1")}
			if _, existed := c.items.LoadOrStore(key, newItem); !existed {
				return 1, nil
			}
			continue
		}

		itm := v.(*item)
		val, err := parseInt64(itm.value)
		if err != nil {
			return 0, fmt.Errorf("value is not an integer")
		}

		val++
		newItem := &item{value: []byte(fmt.Sprintf("%d", val)), expiration: itm.expiration}
		if c.items.CompareAndSwap(key, v, newItem) {
			return val, nil
		}
	}
}

func (c *Cache) Decr(ctx context.Context, key string) (int64, error) {
	for {
		v, loaded := c.items.Load(key)
		if !loaded {
			newItem := &item{value: []byte("-1")}
			if _, existed := c.items.LoadOrStore(key, newItem); !existed {
				return -1, nil
			}
			continue
		}

		itm := v.(*item)
		val, err := parseInt64(itm.value)
		if err != nil {
			return 0, fmt.Errorf("value is not an integer")
		}

		val--
		newItem := &item{value: []byte(fmt.Sprintf("%d", val)), expiration: itm.expiration}
		if c.items.CompareAndSwap(key, v, newItem) {
			return val, nil
		}
	}
}

func parseInt64(b []byte) (int64, error) {
	var val int64
	_, err := fmt.Sscanf(string(b), "%d", &val)
	return val, err
}

func (c *Cache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	v, exists := c.items.Load(key)
	if !exists {
		return fmt.Errorf("key not found: %s", key)
	}

	itm := v.(*item)
	var expiration time.Time
	if ttl > 0 {
		expiration = time.Now().Add(ttl)
	}

	newItem := &item{value: itm.value, expiration: expiration}
	c.items.Store(key, newItem)
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
	now := time.Now()
	c.items.Range(func(k, v any) bool {
		itm := v.(*item)
		if !itm.expiration.IsZero() && now.After(itm.expiration) {
			c.items.Delete(k)
		}
		return true
	})
}
