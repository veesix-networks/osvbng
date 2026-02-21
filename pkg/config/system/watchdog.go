package system

import "time"

type WatchdogConfig struct {
	Enabled       bool                                `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	CheckInterval time.Duration                       `json:"check-interval,omitempty" yaml:"check-interval,omitempty"`
	Targets       map[string]*WatchdogTargetConfig    `json:"targets,omitempty" yaml:"targets,omitempty"`
}

type WatchdogTargetConfig struct {
	Enabled             bool          `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Timeout             time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	FailureThreshold    int           `json:"failure-threshold,omitempty" yaml:"failure-threshold,omitempty"`
	OnFailure           string        `json:"on-failure,omitempty" yaml:"on-failure,omitempty"`
	MinRestartInterval  time.Duration `json:"min-restart-interval,omitempty" yaml:"min-restart-interval,omitempty"`
	ReconnectBackoff    time.Duration `json:"reconnect-backoff,omitempty" yaml:"reconnect-backoff,omitempty"`
	ReconnectMaxBackoff time.Duration `json:"reconnect-max-backoff,omitempty" yaml:"reconnect-max-backoff,omitempty"`
	ReconnectMaxRetries int           `json:"reconnect-max-retries,omitempty" yaml:"reconnect-max-retries,omitempty"`
	Critical            *bool         `json:"critical,omitempty" yaml:"critical,omitempty"`
	FailExitCode        int           `json:"fail-exit-code,omitempty" yaml:"fail-exit-code,omitempty"`
	FailDelay           time.Duration `json:"fail-delay,omitempty" yaml:"fail-delay,omitempty"`
}

func DefaultWatchdogConfig() WatchdogConfig {
	return WatchdogConfig{
		Enabled:       true,
		CheckInterval: 5 * time.Second,
		Targets: map[string]*WatchdogTargetConfig{
			"vpp": {
				Enabled:             true,
				Timeout:             3 * time.Second,
				FailureThreshold:    3,
				OnFailure:           "recover",
				ReconnectBackoff:    1 * time.Second,
				ReconnectMaxBackoff: 30 * time.Second,
				ReconnectMaxRetries: 0,
				Critical:            boolPtr(true),
				FailExitCode:        1,
			},
			"frr": {
				Enabled:             true,
				Timeout:             3 * time.Second,
				FailureThreshold:    3,
				OnFailure:           "warn",
				ReconnectBackoff:    1 * time.Second,
				ReconnectMaxBackoff: 30 * time.Second,
				ReconnectMaxRetries: 0,
				Critical:            boolPtr(false),
			},
		},
	}
}

func boolPtr(b bool) *bool { return &b }
