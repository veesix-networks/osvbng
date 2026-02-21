package watchdog

import (
	"context"
	"log/slog"
	"sync"

	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/logger"
)

type StateProvider interface {
	GetAllStates() []StateInfo
	IsReady() bool
}

type Watchdog struct {
	*component.Base
	logger  *slog.Logger
	runners map[string]*targetRunner
	mu      sync.RWMutex
}

func New() *Watchdog {
	return &Watchdog{
		Base:    component.NewBase("watchdog"),
		logger:  logger.Get("watchdog"),
		runners: make(map[string]*targetRunner),
	}
}

func (w *Watchdog) Register(target Target, config RunnerConfig) {
	w.mu.Lock()
	defer w.mu.Unlock()

	name := target.Name()
	if _, exists := w.runners[name]; exists {
		w.logger.Warn("target already registered, replacing", "target", name)
	}

	w.runners[name] = newTargetRunner(target, config, w.logger)
	w.logger.Info("registered target", "target", name, "critical", target.Critical(), "action", config.OnFailure)
}

func (w *Watchdog) Start(ctx context.Context) error {
	w.StartContext(ctx)
	w.logger.Info("starting watchdog")

	w.mu.RLock()
	defer w.mu.RUnlock()

	for _, runner := range w.runners {
		runner.start(w.Ctx)
	}

	return nil
}

func (w *Watchdog) Stop(ctx context.Context) error {
	w.logger.Info("stopping watchdog")

	w.mu.RLock()
	defer w.mu.RUnlock()

	for _, runner := range w.runners {
		runner.stop()
	}

	w.StopContext()
	return nil
}

func (w *Watchdog) GetState(name string) (StateInfo, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	runner, ok := w.runners[name]
	if !ok {
		return StateInfo{}, false
	}
	return runner.getStateInfo(), true
}

func (w *Watchdog) GetAllStates() []StateInfo {
	w.mu.RLock()
	defer w.mu.RUnlock()

	states := make([]StateInfo, 0, len(w.runners))
	for _, runner := range w.runners {
		states = append(states, runner.getStateInfo())
	}
	return states
}

func (w *Watchdog) IsHealthy() bool {
	return true
}

func (w *Watchdog) IsReady() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()

	for _, runner := range w.runners {
		if runner.target.Critical() && runner.state.Load() != StateUp {
			return false
		}
	}
	return true
}
