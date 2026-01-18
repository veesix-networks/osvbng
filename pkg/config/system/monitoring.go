package system

import "time"

type MonitoringConfig struct {
	DisabledCollectors []string      `json:"disabled_collectors,omitempty" yaml:"disabled_collectors,omitempty"`
	CollectInterval    time.Duration `json:"collect_interval,omitempty" yaml:"collect_interval,omitempty"`
}
