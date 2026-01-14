package cache

import (
	"context"
	"time"
)

type Cache interface {
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Get(ctx context.Context, key string) ([]byte, error)
	GetAll(ctx context.Context, pattern string) (map[string][]byte, error)
	Delete(ctx context.Context, key string) error
	Scan(ctx context.Context, cursor uint64, pattern string, count int64) ([]string, uint64, error)
	Incr(ctx context.Context, key string) (int64, error)
	Decr(ctx context.Context, key string) (int64, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error
	Close() error
}
