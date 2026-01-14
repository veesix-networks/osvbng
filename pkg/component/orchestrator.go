package component

import (
	"context"
	"fmt"
	"sync"
)

type Orchestrator struct {
	components []Component
	mu         sync.RWMutex
}

func NewOrchestrator() *Orchestrator {
	return &Orchestrator{
		components: make([]Component, 0),
	}
}

func (o *Orchestrator) Register(comp Component) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.components = append(o.components, comp)
}

func (o *Orchestrator) Start(ctx context.Context) error {
	o.mu.RLock()
	defer o.mu.RUnlock()

	for _, comp := range o.components {
		if err := comp.Start(ctx); err != nil {
			return fmt.Errorf("failed to start %s: %w", comp.Name(), err)
		}
	}
	return nil
}

func (o *Orchestrator) Stop(ctx context.Context) error {
	o.mu.RLock()
	defer o.mu.RUnlock()

	for i := len(o.components) - 1; i >= 0; i-- {
		comp := o.components[i]
		if err := comp.Stop(ctx); err != nil {
			return fmt.Errorf("failed to stop %s: %w", comp.Name(), err)
		}
	}
	return nil
}
