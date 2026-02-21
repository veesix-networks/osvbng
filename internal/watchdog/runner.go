package watchdog

import (
	"context"
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

type FailureAction string

const (
	ActionRecover FailureAction = "recover"
	ActionRestart FailureAction = "restart"
	ActionWarn    FailureAction = "warn"
	ActionFail    FailureAction = "fail"
)

type RunnerConfig struct {
	CheckInterval       time.Duration
	Timeout             time.Duration
	FailureThreshold    int
	OnFailure           FailureAction
	MinRestartInterval  time.Duration
	ReconnectBackoff    time.Duration
	ReconnectMaxBackoff time.Duration
	ReconnectMaxRetries int
	FailExitCode        int
	FailDelay           time.Duration
}

type targetRunner struct {
	target Target
	config RunnerConfig
	logger *slog.Logger

	state           atomicState
	lastCheck       atomic.Pointer[HealthResult]
	consecFailures  atomic.Int64
	totalFailures   atomic.Int64
	totalRecoveries atomic.Int64
	totalRestarts   atomic.Int64
	lastStateChange atomic.Pointer[time.Time]
	upSince         atomic.Pointer[time.Time]
	lastRestart     atomic.Pointer[time.Time]

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func newTargetRunner(target Target, config RunnerConfig, logger *slog.Logger) *targetRunner {
	r := &targetRunner{
		target: target,
		config: config,
		logger: logger.With("target", target.Name()),
	}
	r.state.Store(StateInit)
	now := time.Now()
	r.lastStateChange.Store(&now)
	return r
}

func (r *targetRunner) start(ctx context.Context) {
	ctx, r.cancel = context.WithCancel(ctx)
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.run(ctx)
	}()
}

func (r *targetRunner) stop() {
	if r.cancel != nil {
		r.cancel()
	}
	r.wg.Wait()
}

func (r *targetRunner) getStateInfo() StateInfo {
	info := StateInfo{
		Name:            r.target.Name(),
		State:           r.state.Load().String(),
		Critical:        r.target.Critical(),
		ConsecFailures:  r.consecFailures.Load(),
		TotalFailures:   r.totalFailures.Load(),
		TotalRecoveries: r.totalRecoveries.Load(),
		TotalRestarts:   r.totalRestarts.Load(),
	}

	if lc := r.lastCheck.Load(); lc != nil {
		info.LastCheck = lc
	}
	if t := r.lastStateChange.Load(); t != nil {
		info.LastStateChange = *t
	}
	if t := r.upSince.Load(); t != nil {
		info.Uptime = time.Since(*t).Truncate(time.Second).String()
	}

	return info
}

func (r *targetRunner) run(ctx context.Context) {
	ticker := time.NewTicker(r.config.CheckInterval)
	defer ticker.Stop()

	r.doCheck(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.doCheck(ctx)
		}
	}
}

func (r *targetRunner) doCheck(ctx context.Context) {
	checkCtx, cancel := context.WithTimeout(ctx, r.config.Timeout)
	defer cancel()

	result := r.target.Check(checkCtx)
	r.lastCheck.Store(result)

	if result.Healthy {
		r.handleSuccess()
	} else {
		r.handleFailure(ctx, result)
	}
}

func (r *targetRunner) handleSuccess() {
	prev := r.state.Load()
	if prev != StateUp {
		r.setState(StateUp)
		now := time.Now()
		r.upSince.Store(&now)
		r.logger.Info("target is UP", "previous_state", prev.String())
		r.target.OnUp()
	}
	r.consecFailures.Store(0)
}

func (r *targetRunner) handleFailure(ctx context.Context, result *HealthResult) {
	failures := r.consecFailures.Add(1)
	r.totalFailures.Add(1)

	if int(failures) < r.config.FailureThreshold {
		r.logger.Warn("health check failed", "failures", failures, "threshold", r.config.FailureThreshold, "error", result.Error)
		return
	}

	if r.state.Load() == StateUp || r.state.Load() == StateInit {
		r.setState(StateDown)
		r.upSince.Store(nil)
		r.logger.Error("target is DOWN", "failures", failures, "error", result.Error)
		r.target.OnDown()
	}

	r.dispatchAction(ctx)
}

func (r *targetRunner) dispatchAction(ctx context.Context) {
	switch r.config.OnFailure {
	case ActionRecover:
		r.doRecover(ctx)
	case ActionRestart:
		r.doRestart(ctx)
	case ActionWarn:
		r.logger.Warn("target down, action=warn (no recovery)")
	case ActionFail:
		r.doFail()
	default:
		r.logger.Warn("unknown failure action", "action", r.config.OnFailure)
	}
}

func (r *targetRunner) doRecover(ctx context.Context) {
	if r.state.Load() == StateReconnecting || r.state.Load() == StateRecovering {
		return
	}
	r.setState(StateReconnecting)

	backoff := r.config.ReconnectBackoff
	maxRetries := r.config.ReconnectMaxRetries

	for attempt := 0; maxRetries <= 0 || attempt < maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		r.logger.Info("attempting reconnection", "attempt", attempt+1)

		if err := r.target.Connect(ctx); err != nil {
			r.logger.Warn("reconnection failed", "attempt", attempt+1, "error", err)
			sleep := r.backoffDuration(backoff, attempt)
			select {
			case <-ctx.Done():
				return
			case <-time.After(sleep):
			}
			continue
		}

		r.setState(StateRecovering)
		r.logger.Info("reconnected, running recovery")

		if err := r.target.Recover(ctx); err != nil {
			r.logger.Error("recovery failed", "error", err)
			r.setState(StateDown)
			sleep := r.backoffDuration(backoff, attempt)
			r.logger.Info("waiting before retry", "delay", sleep)
			select {
			case <-ctx.Done():
				return
			case <-time.After(sleep):
			}
			continue
		}

		r.totalRecoveries.Add(1)
		r.consecFailures.Store(0)
		r.setState(StateUp)
		now := time.Now()
		r.upSince.Store(&now)
		r.logger.Info("target recovered")
		r.target.OnUp()
		return
	}

	r.logger.Error("max reconnection retries exhausted")
	r.setState(StateFailed)
}

func (r *targetRunner) doRestart(ctx context.Context) {
	if r.state.Load() == StateReconnecting || r.state.Load() == StateRecovering {
		return
	}

	if err := r.target.Connect(ctx); err == nil {
		r.logger.Info("target reachable, skipping restart")
		r.doRecover(ctx)
		return
	}

	if lr := r.lastRestart.Load(); lr != nil && r.config.MinRestartInterval > 0 {
		since := time.Since(*lr)
		if since < r.config.MinRestartInterval {
			wait := r.config.MinRestartInterval - since
			r.logger.Warn("restart rate limited", "wait", wait)
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
		}
	}

	r.logger.Info("restarting target")
	now := time.Now()
	r.lastRestart.Store(&now)
	r.totalRestarts.Add(1)

	if err := r.target.Restart(ctx); err != nil {
		r.logger.Error("restart failed", "error", err)
	} else {
		r.logger.Info("restart succeeded")
	}

	r.logger.Info("waiting for target to become reachable")
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return
		case <-time.After(500 * time.Millisecond):
		}
		if err := r.target.Connect(ctx); err == nil {
			r.logger.Info("target reachable after restart")
			break
		}
	}

	r.doRecover(ctx)
}

func (r *targetRunner) doFail() {
	r.setState(StateFailed)
	r.logger.Error("target failed, requesting process exit", "exit_code", r.config.FailExitCode)
	if r.config.FailDelay > 0 {
		time.Sleep(r.config.FailDelay)
	}
}

func (r *targetRunner) setState(s TargetState) {
	r.state.Store(s)
	now := time.Now()
	r.lastStateChange.Store(&now)
}

func (r *targetRunner) backoffDuration(base time.Duration, attempt int) time.Duration {
	d := time.Duration(float64(base) * math.Pow(2, float64(attempt)))
	if d > r.config.ReconnectMaxBackoff {
		d = r.config.ReconnectMaxBackoff
	}
	return d
}
