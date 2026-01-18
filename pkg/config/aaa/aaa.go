package aaa

type AAAConfig struct {
	Provider      string      `json:"provider,omitempty" yaml:"provider,omitempty"`
	NASIdentifier string      `json:"nas_identifier,omitempty" yaml:"nas_identifier,omitempty"`
	NASIP         string      `json:"nas_ip,omitempty" yaml:"nas_ip,omitempty"`
	Policy        []AAAPolicy `json:"policy,omitempty" yaml:"policy,omitempty"`
}

type AAAPolicy struct {
	Name                  string `json:"name" yaml:"name"`
	Format                string `json:"format,omitempty" yaml:"format,omitempty"`
	Type                  string `json:"type,omitempty" yaml:"type,omitempty"`
	MaxConcurrentSessions int    `json:"max_concurrent_sessions,omitempty" yaml:"max_concurrent_sessions,omitempty"`
}

func (a *AAAConfig) GetPolicy(name string) *AAAPolicy {
	for i := range a.Policy {
		if a.Policy[i].Name == name {
			return &a.Policy[i]
		}
	}
	return nil
}
