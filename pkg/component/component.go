package component

import "context"

type Component interface {
	Name() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

type PluginConfig[T any] interface {
	GetConfig() *T
}
