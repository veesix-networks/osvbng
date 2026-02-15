package protocols

type MPLSConfig struct {
	Enabled        bool   `json:"enabled" yaml:"enabled"`
	PlatformLabels uint32 `json:"platform-labels,omitempty" yaml:"platform-labels,omitempty"`
}
