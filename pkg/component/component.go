package component

import "context"

type Component interface {
	Name() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

type ReadyNotifier interface {
	Ready() <-chan struct{}
}

type PluginConfig[T any] interface {
	GetConfig() *T
}
