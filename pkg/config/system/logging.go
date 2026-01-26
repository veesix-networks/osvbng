package system

import "github.com/veesix-networks/osvbng/pkg/logger"

type LoggingConfig struct {
	Format     string                     `json:"format,omitempty" yaml:"format,omitempty"`
	Level      logger.LogLevel            `json:"level,omitempty" yaml:"level,omitempty"`
	Components map[string]logger.LogLevel `json:"components,omitempty" yaml:"components,omitempty"`
}
