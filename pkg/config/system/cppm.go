package system

type CPPMConfig struct {
	Dataplane    *CPPMPlaneConfig `json:"dataplane,omitempty" yaml:"dataplane,omitempty"`
	Controlplane *CPPMPlaneConfig `json:"controlplane,omitempty" yaml:"controlplane,omitempty"`
}

type CPPMPlaneConfig struct {
	Policer map[string]*CPPMPolicerConfig `json:"policer,omitempty" yaml:"policer,omitempty"`
}

type CPPMPolicerConfig struct {
	Rate  float64 `json:"rate" yaml:"rate"`
	Burst uint32  `json:"burst" yaml:"burst"`
}
