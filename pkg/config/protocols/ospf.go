package protocols

type OSPFConfig struct {
	RouterID string        `json:"router-id" yaml:"router-id"`
	Networks []OSPFNetwork `json:"networks,omitempty" yaml:"networks,omitempty"`
}

type OSPFNetwork struct {
	Prefix string `json:"prefix" yaml:"prefix"`
	Area   string `json:"area" yaml:"area"`
}
