package system

import (
	"context"

	"github.com/veesix-networks/osvbng/internal/watchdog"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/telemetry"
)

type WatchdogHandler struct {
	deps *deps.ShowDeps
}

type WatchdogTargetInfo struct {
	Name            string  `json:"name"                  metric:"label"`
	State           string  `json:"state"                 metric:"label"`
	Critical        bool    `json:"critical"`
	LastCheckOK     bool    `json:"last-check-ok"         metric:"name=watchdog.target.up,type=gauge,help=1 if the most recent health check succeeded."`
	LastCheckError  string  `json:"last-check-error,omitempty"`
	LastCheckMs     float64 `json:"last-check-ms"         metric:"name=watchdog.target.health_check_ms,type=gauge,help=Duration of the most recent health check."`
	ConsecFailures  int64   `json:"consecutive-failures"  metric:"name=watchdog.target.consecutive_failures,type=gauge,help=Consecutive failed health checks."`
	TotalFailures   int64   `json:"total-failures"        metric:"name=watchdog.target.failures,type=counter,help=Total health-check failures."`
	TotalRecoveries int64   `json:"total-recoveries"      metric:"name=watchdog.target.recoveries,type=counter,help=Total successful recoveries."`
	TotalRestarts   int64   `json:"total-restarts"        metric:"name=watchdog.target.restarts,type=counter,help=Total target restarts."`
	LastStateChange string  `json:"last-state-change"`
	Uptime          string  `json:"uptime,omitempty"`
}

func init() {
	show.RegisterFactory(func(deps *deps.ShowDeps) show.ShowHandler {
		return &WatchdogHandler{deps: deps}
	})
	telemetry.RegisterMetric[WatchdogTargetInfo](paths.SystemWatchdog)
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

func (h *WatchdogHandler) Summary() string {
	return "Show watchdog status"
}

func (h *WatchdogHandler) Description() string {
	return "Display the watchdog health check status for monitored components."
}
