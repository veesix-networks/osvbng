package opdb

import "context"

type Store interface {
	Put(ctx context.Context, namespace, key string, value []byte) error
	Delete(ctx context.Context, namespace, key string) error
	Load(ctx context.Context, namespace string, fn LoadFunc) error
	Count(ctx context.Context, namespace string) (int, error)
	Clear(ctx context.Context, namespace string) error
	Stats() Stats
	Close() error
}

type LoadFunc func(key string, value []byte) error

type Stats struct {
	Puts    uint64
	Deletes uint64
	Loads   uint64
	Clears  uint64
}

const (
	NamespaceIPoESessions  = "ipoe_sessions"
	NamespacePPPoESessions = "pppoe_sessions"
)
