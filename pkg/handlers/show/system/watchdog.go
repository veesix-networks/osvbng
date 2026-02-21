package system

import (
	"context"

	"github.com/veesix-networks/osvbng/internal/watchdog"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/state"
	statepaths "github.com/veesix-networks/osvbng/pkg/state/paths"
)

type WatchdogHandler struct {
	deps *deps.ShowDeps
}

type WatchdogTargetInfo struct {
	Name            string  `json:"name"`
	State           string  `json:"state"`
	Critical        bool    `json:"critical"`
	LastCheckOK     bool    `json:"last-check-ok"`
	LastCheckError  string  `json:"last-check-error,omitempty"`
	LastCheckMs     float64 `json:"last-check-ms"`
	ConsecFailures  int64   `json:"consecutive-failures"`
	TotalFailures   int64   `json:"total-failures"`
	TotalRecoveries int64   `json:"total-recoveries"`
	TotalRestarts   int64   `json:"total-restarts"`
	LastStateChange string  `json:"last-state-change"`
	Uptime          string  `json:"uptime,omitempty"`
}

func init() {
	state.RegisterMetric(statepaths.SystemWatchdog, paths.SystemWatchdog)

	show.RegisterFactory(func(deps *deps.ShowDeps) show.ShowHandler {
		return &WatchdogHandler{deps: deps}
	})
}

func (h *WatchdogHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.deps.Watchdog == nil {
		return []WatchdogTargetInfo{}, nil
	}

	states := h.deps.Watchdog.GetAllStates()
	result := make([]WatchdogTargetInfo, 0, len(states))

	for _, s := range states {
		info := WatchdogTargetInfo{
			Name:            s.Name,
			State:           s.State,
			Critical:        s.Critical,
			ConsecFailures:  s.ConsecFailures,
			TotalFailures:   s.TotalFailures,
			TotalRecoveries: s.TotalRecoveries,
			TotalRestarts:   s.TotalRestarts,
			LastStateChange: s.LastStateChange.Format("2006-01-02T15:04:05Z"),
			Uptime:          s.Uptime,
		}

		if s.LastCheck != nil {
			info.LastCheckOK = s.LastCheck.Healthy
			info.LastCheckMs = s.LastCheck.LatencyMs
			if s.LastCheck.Error != nil {
				info.LastCheckError = s.LastCheck.Error.Error()
			}
		}

		result = append(result, info)
	}

	return result, nil
}

func (h *WatchdogHandler) PathPattern() paths.Path {
	return paths.SystemWatchdog
}

func (h *WatchdogHandler) Dependencies() []paths.Path {
	return nil
}

func (h *WatchdogHandler) OutputType() interface{} {
	return &[]WatchdogTargetInfo{}
}

var _ show.ShowHandler = (*WatchdogHandler)(nil)
var _ watchdog.StateProvider = (*watchdog.Watchdog)(nil)
