package protocols

type OSPFNetworkType string

const (
	OSPFNetworkBroadcast         OSPFNetworkType = "broadcast"
	OSPFNetworkNonBroadcast      OSPFNetworkType = "non-broadcast"
	OSPFNetworkPointToMultipoint OSPFNetworkType = "point-to-multipoint"
	OSPFNetworkPointToPoint      OSPFNetworkType = "point-to-point"
)

func (n OSPFNetworkType) Valid() bool {
	switch n {
	case OSPFNetworkBroadcast, OSPFNetworkNonBroadcast, OSPFNetworkPointToMultipoint, OSPFNetworkPointToPoint:
		return true
	}
	return false
}

type OSPFAuthMode string

const (
	OSPFAuthNone          OSPFAuthMode = ""
	OSPFAuthMessageDigest OSPFAuthMode = "message-digest"
)

func (a OSPFAuthMode) Valid() bool {
	switch a {
	case OSPFAuthNone, OSPFAuthMessageDigest:
		return true
	}
	return false
}

type OSPFConfig struct {
	Enabled              bool                 `json:"enabled" yaml:"enabled"`
	RouterID             string               `json:"router-id,omitempty" yaml:"router-id,omitempty"`
	Areas                map[string]*OSPFArea `json:"areas,omitempty" yaml:"areas,omitempty"`
	Redistribute         *OSPFRedistribute    `json:"redistribute,omitempty" yaml:"redistribute,omitempty"`
	DefaultInformation   *OSPFDefaultInfo     `json:"default-information,omitempty" yaml:"default-information,omitempty"`
	LogAdjacencyChanges  bool                 `json:"log-adjacency-changes,omitempty" yaml:"log-adjacency-changes,omitempty"`
	AutoCostRefBandwidth uint32               `json:"auto-cost-reference-bandwidth,omitempty" yaml:"auto-cost-reference-bandwidth,omitempty"`
	MaximumPaths         uint32               `json:"maximum-paths,omitempty" yaml:"maximum-paths,omitempty"`
	DefaultMetric        uint32               `json:"default-metric,omitempty" yaml:"default-metric,omitempty"`
	Distance             uint32               `json:"distance,omitempty" yaml:"distance,omitempty"`
}

type OSPFArea struct {
	Interfaces     map[string]*OSPFInterfaceConfig `json:"interfaces,omitempty" yaml:"interfaces,omitempty"`
	Authentication OSPFAuthMode                     `json:"authentication,omitempty" yaml:"authentication,omitempty"`
}

type OSPFInterfaceConfig struct {
	Passive       bool   `json:"passive,omitempty" yaml:"passive,omitempty"`
	Cost          uint32 `json:"cost,omitempty" yaml:"cost,omitempty"`
	Network       OSPFNetworkType `json:"network,omitempty" yaml:"network,omitempty"`
	BFD           bool            `json:"bfd,omitempty" yaml:"bfd,omitempty"`
	HelloInterval uint32 `json:"hello-interval,omitempty" yaml:"hello-interval,omitempty"`
	DeadInterval  uint32 `json:"dead-interval,omitempty" yaml:"dead-interval,omitempty"`
	MTUIgnore     bool   `json:"mtu-ignore,omitempty" yaml:"mtu-ignore,omitempty"`
	Priority      uint32 `json:"priority,omitempty" yaml:"priority,omitempty"`
}

type OSPFRedistribute struct {
	Connected bool `json:"connected,omitempty" yaml:"connected,omitempty"`
	Static    bool `json:"static,omitempty" yaml:"static,omitempty"`
	BGP       bool `json:"bgp,omitempty" yaml:"bgp,omitempty"`
}

type OSPFDefaultInfo struct {
	Originate  bool   `json:"originate" yaml:"originate"`
	Always     bool   `json:"always,omitempty" yaml:"always,omitempty"`
	Metric     uint32 `json:"metric,omitempty" yaml:"metric,omitempty"`
	MetricType uint32 `json:"metric-type,omitempty" yaml:"metric-type,omitempty"`
}
