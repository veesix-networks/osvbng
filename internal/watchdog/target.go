package watchdog

import (
	"context"
	"sync/atomic"
	"time"
)

type Target interface {
	Name() string
	Check(ctx context.Context) *HealthResult
	Connect(ctx context.Context) error
	Disconnect() error
	Restart(ctx context.Context) error
	OnDown()
	OnUp()
	Recover(ctx context.Context) error
	Critical() bool
}

type HealthResult struct {
	Healthy   bool          `json:"healthy"`
	Error     error         `json:"-"`
	ErrorStr  string        `json:"error,omitempty"`
	Latency   time.Duration `json:"-"`
	LatencyMs float64       `json:"latency-ms"`
	Timestamp time.Time     `json:"timestamp"`
}

func NewHealthResult(healthy bool, err error, latency time.Duration) *HealthResult {
	r := &HealthResult{
		Healthy:   healthy,
		Error:     err,
		Latency:   latency,
		LatencyMs: float64(latency.Microseconds()) / 1000.0,
		Timestamp: time.Now(),
	}
	if err != nil {
		r.ErrorStr = err.Error()
	}
	return r
}

type TargetState int32

const (
	StateInit TargetState = iota
	StateUp
	StateDown
	StateReconnecting
	StateRecovering
	StateFailed
)

func (s TargetState) String() string {
	switch s {
	case StateInit:
		return "init"
	case StateUp:
		return "up"
	case StateDown:
		return "down"
	case StateReconnecting:
		return "reconnecting"
	case StateRecovering:
		return "recovering"
	case StateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

type StateInfo struct {
	Name             string        `json:"name"`
	State            string        `json:"state"`
	Critical         bool          `json:"critical"`
	LastCheck        *HealthResult `json:"last-check,omitempty"`
	ConsecFailures   int64         `json:"consecutive-failures"`
	TotalFailures    int64         `json:"total-failures"`
	TotalRecoveries  int64         `json:"total-recoveries"`
	TotalRestarts    int64         `json:"total-restarts"`
	LastStateChange  time.Time     `json:"last-state-change"`
	Uptime           string        `json:"uptime,omitempty"`
}

type atomicState struct {
	val atomic.Int32
}

func (s *atomicState) Load() TargetState {
	return TargetState(s.val.Load())
}

func (s *atomicState) Store(state TargetState) {
	s.val.Store(int32(state))
}
