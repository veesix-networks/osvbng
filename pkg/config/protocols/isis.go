package protocols

type ISISIsType string

const (
	ISISIsTypeLevel1     ISISIsType = "level-1"
	ISISIsTypeLevel1L2   ISISIsType = "level-1-2"
	ISISIsTypeLevel2Only ISISIsType = "level-2-only"
)

func (t ISISIsType) Valid() bool {
	switch t {
	case ISISIsTypeLevel1, ISISIsTypeLevel1L2, ISISIsTypeLevel2Only:
		return true
	}
	return false
}

type ISISMetricStyle string

const (
	ISISMetricStyleNarrow     ISISMetricStyle = "narrow"
	ISISMetricStyleTransition ISISMetricStyle = "transition"
	ISISMetricStyleWide       ISISMetricStyle = "wide"
)

func (m ISISMetricStyle) Valid() bool {
	switch m {
	case ISISMetricStyleNarrow, ISISMetricStyleTransition, ISISMetricStyleWide:
		return true
	}
	return false
}

type ISISCircuitType string

const (
	ISISCircuitTypeLevel1   ISISCircuitType = "level-1"
	ISISCircuitTypeLevel1L2 ISISCircuitType = "level-1-2"
	ISISCircuitTypeLevel2   ISISCircuitType = "level-2"
)

func (c ISISCircuitType) Valid() bool {
	switch c {
	case ISISCircuitTypeLevel1, ISISCircuitTypeLevel1L2, ISISCircuitTypeLevel2:
		return true
	}
	return false
}

type ISISConfig struct {
	Enabled             bool                             `json:"enabled" yaml:"enabled"`
	NET                 string                           `json:"net,omitempty" yaml:"net,omitempty"`
	IsType              ISISIsType                       `json:"is-type,omitempty" yaml:"is-type,omitempty"`
	MetricStyle         ISISMetricStyle                  `json:"metric-style,omitempty" yaml:"metric-style,omitempty"`
	LogAdjacencyChanges bool                             `json:"log-adjacency-changes,omitempty" yaml:"log-adjacency-changes,omitempty"`
	DynamicHostname     bool                             `json:"dynamic-hostname,omitempty" yaml:"dynamic-hostname,omitempty"`
	SetOverloadBit      bool                             `json:"set-overload-bit,omitempty" yaml:"set-overload-bit,omitempty"`
	LSPMTU              uint32                           `json:"lsp-mtu,omitempty" yaml:"lsp-mtu,omitempty"`
	LSPGenInterval      uint32                           `json:"lsp-gen-interval,omitempty" yaml:"lsp-gen-interval,omitempty"`
	LSPRefreshInterval  uint32                           `json:"lsp-refresh-interval,omitempty" yaml:"lsp-refresh-interval,omitempty"`
	MaxLSPLifetime      uint32                           `json:"max-lsp-lifetime,omitempty" yaml:"max-lsp-lifetime,omitempty"`
	SPFInterval         uint32                           `json:"spf-interval,omitempty" yaml:"spf-interval,omitempty"`
	AreaPassword        string                           `json:"area-password,omitempty" yaml:"area-password,omitempty"`
	DomainPassword      string                           `json:"domain-password,omitempty" yaml:"domain-password,omitempty"`
	Redistribute        *ISISRedistribute                `json:"redistribute,omitempty" yaml:"redistribute,omitempty"`
	DefaultInformation  *ISISDefaultInfo                 `json:"default-information,omitempty" yaml:"default-information,omitempty"`
	Interfaces          map[string]*ISISInterfaceConfig   `json:"interfaces,omitempty" yaml:"interfaces,omitempty"`
}

type ISISInterfaceConfig struct {
	Passive         bool   `json:"passive,omitempty" yaml:"passive,omitempty"`
	Metric          uint32 `json:"metric,omitempty" yaml:"metric,omitempty"`
	Network         string `json:"network,omitempty" yaml:"network,omitempty"`
	BFD             bool   `json:"bfd,omitempty" yaml:"bfd,omitempty"`
	CircuitType     ISISCircuitType `json:"circuit-type,omitempty" yaml:"circuit-type,omitempty"`
	HelloInterval   uint32 `json:"hello-interval,omitempty" yaml:"hello-interval,omitempty"`
	HelloMultiplier uint32 `json:"hello-multiplier,omitempty" yaml:"hello-multiplier,omitempty"`
	Priority        uint32 `json:"priority,omitempty" yaml:"priority,omitempty"`
}

type ISISRedistribute struct {
	IPv4Connected bool `json:"ipv4-connected,omitempty" yaml:"ipv4-connected,omitempty"`
	IPv4Static    bool `json:"ipv4-static,omitempty" yaml:"ipv4-static,omitempty"`
	IPv6Connected bool `json:"ipv6-connected,omitempty" yaml:"ipv6-connected,omitempty"`
	IPv6Static    bool `json:"ipv6-static,omitempty" yaml:"ipv6-static,omitempty"`
}

type ISISDefaultInfo struct {
	Originate bool `json:"originate" yaml:"originate"`
}
