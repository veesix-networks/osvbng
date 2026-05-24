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

// ReadinessReporter is the richer surface implemented by *Base for
// components that participate in the recovery state machine. The
// orchestrator (and the /api/show/system/recovery/status handler) use
// it to aggregate per-component state for operator visibility, distinct
// from the one-shot Ready() channel that gates Orchestrator.WaitReady.
type ReadinessReporter interface {
	Name() string
	ReadyState() ReadyState
	Progress() *RestoreProgress
}

type PluginConfig[T any] interface {
	GetConfig() *T
}
