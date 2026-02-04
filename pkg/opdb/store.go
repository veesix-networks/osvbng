package opdb

import "context"

type Store interface {
	Put(ctx context.Context, namespace, key string, value []byte) error
	Delete(ctx context.Context, namespace, key string) error
	Load(ctx context.Context, namespace string, fn LoadFunc) error
	Clear(ctx context.Context, namespace string) error
	Close() error
}

type LoadFunc func(key string, value []byte) error

const (
	NamespaceDHCPv4Sessions = "dhcpv4_sessions"
	NamespaceDHCPv6Sessions = "dhcpv6_sessions"
	NamespacePPPoESessions  = "pppoe_sessions"
)
