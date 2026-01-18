package system

type LoggingConfig struct {
	Format     string            `json:"format,omitempty" yaml:"format,omitempty"`
	Level      string            `json:"level,omitempty" yaml:"level,omitempty"`
	Components map[string]string `json:"components,omitempty" yaml:"components,omitempty"`
}
