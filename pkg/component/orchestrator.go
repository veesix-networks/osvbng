// Copyright 2025 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package component

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type Orchestrator struct {
	components []Component
	plugins    map[string]bool
	mu         sync.RWMutex
	logger     *slog.Logger
}

func NewOrchestrator() *Orchestrator {
	return &Orchestrator{
		components: make([]Component, 0),
		plugins:    make(map[string]bool),
		logger:     slog.Default(),
	}
}

func (o *Orchestrator) Register(comp Component) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.components = append(o.components, comp)
}

func (o *Orchestrator) RegisterPlugin(comp Component) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.components = append(o.components, comp)
	o.plugins[comp.Name()] = true
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

func (o *Orchestrator) WaitReady(ctx context.Context, pluginTimeout time.Duration) error {
	o.mu.RLock()
	defer o.mu.RUnlock()

	for _, comp := range o.components {
		if o.plugins[comp.Name()] {
			continue
		}
		rn, ok := comp.(ReadyNotifier)
		if !ok {
			continue
		}
		select {
		case <-rn.Ready():
		case <-ctx.Done():
			return fmt.Errorf("context cancelled waiting for %s to be ready", comp.Name())
		}
	}

	pluginCtx, cancel := context.WithTimeout(ctx, pluginTimeout)
	defer cancel()

	var wg sync.WaitGroup
	for _, comp := range o.components {
		if !o.plugins[comp.Name()] {
			continue
		}
		rn, ok := comp.(ReadyNotifier)
		if !ok {
			continue
		}
		wg.Add(1)
		go func(name string, rn ReadyNotifier) {
			defer wg.Done()
			select {
			case <-rn.Ready():
				o.logger.Info("Plugin ready", "name", name)
			case <-pluginCtx.Done():
				o.logger.Warn("Plugin did not signal ready in time, continuing", "name", name, "timeout", pluginTimeout)
			}
		}(comp.Name(), rn)
	}
	wg.Wait()

	return nil
}

// ReadinessSnapshot returns a per-component snapshot of every registered
// component that implements ReadinessReporter (i.e. embeds *Base). Used by
// the /api/show/system/recovery/status handler and the Prometheus
// component-readiness gauge for operator visibility into the recovery
// lifecycle.
func (o *Orchestrator) ReadinessSnapshot() map[string]ComponentReadiness {
	o.mu.RLock()
	defer o.mu.RUnlock()

	out := make(map[string]ComponentReadiness, len(o.components))
	for _, comp := range o.components {
		rr, ok := comp.(ReadinessReporter)
		if !ok {
			continue
		}
		out[rr.Name()] = ComponentReadiness{
			State:    rr.ReadyState().String(),
			Progress: rr.Progress(),
		}
	}
	return out
}

// ComponentReadiness is one row of the recovery-status API payload.
type ComponentReadiness struct {
	State    string           `json:"state"`
	Progress *RestoreProgress `json:"progress,omitempty"`
}

// AllReady reports whether every registered component that exposes a
// ReadinessReporter is in StateReady. Components that do not embed *Base
// are treated as always-ready (they have no recovery lifecycle).
func (o *Orchestrator) AllReady() bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	for _, comp := range o.components {
		rr, ok := comp.(ReadinessReporter)
		if !ok {
			continue
		}
		if rr.ReadyState() != StateReady {
			return false
		}
	}
	return true
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
