package protocols

type OSPF6NetworkType string

const (
	OSPF6NetworkBroadcast         OSPF6NetworkType = "broadcast"
	OSPF6NetworkPointToMultipoint OSPF6NetworkType = "point-to-multipoint"
	OSPF6NetworkPointToPoint      OSPF6NetworkType = "point-to-point"
)

func (n OSPF6NetworkType) Valid() bool {
	switch n {
	case OSPF6NetworkBroadcast, OSPF6NetworkPointToMultipoint, OSPF6NetworkPointToPoint:
		return true
	}
	return false
}

type OSPF6Config struct {
	Enabled              bool                  `json:"enabled" yaml:"enabled"`
	RouterID             string                `json:"router-id,omitempty" yaml:"router-id,omitempty"`
	Areas                map[string]*OSPF6Area `json:"areas,omitempty" yaml:"areas,omitempty"`
	Redistribute         *OSPF6Redistribute    `json:"redistribute,omitempty" yaml:"redistribute,omitempty"`
	DefaultInformation   *OSPF6DefaultInfo     `json:"default-information,omitempty" yaml:"default-information,omitempty"`
	LogAdjacencyChanges  bool                  `json:"log-adjacency-changes,omitempty" yaml:"log-adjacency-changes,omitempty"`
	AutoCostRefBandwidth uint32                `json:"auto-cost-reference-bandwidth,omitempty" yaml:"auto-cost-reference-bandwidth,omitempty"`
	MaximumPaths         uint32                `json:"maximum-paths,omitempty" yaml:"maximum-paths,omitempty"`
	Distance             uint32                `json:"distance,omitempty" yaml:"distance,omitempty"`
}

type OSPF6Area struct {
	Interfaces map[string]*OSPF6InterfaceConfig `json:"interfaces,omitempty" yaml:"interfaces,omitempty"`
}

type OSPF6InterfaceConfig struct {
	Passive       bool   `json:"passive,omitempty" yaml:"passive,omitempty"`
	Cost          uint32 `json:"cost,omitempty" yaml:"cost,omitempty"`
	Network       OSPF6NetworkType `json:"network,omitempty" yaml:"network,omitempty"`
	BFD           bool             `json:"bfd,omitempty" yaml:"bfd,omitempty"`
	HelloInterval uint32 `json:"hello-interval,omitempty" yaml:"hello-interval,omitempty"`
	DeadInterval  uint32 `json:"dead-interval,omitempty" yaml:"dead-interval,omitempty"`
	MTUIgnore     bool   `json:"mtu-ignore,omitempty" yaml:"mtu-ignore,omitempty"`
	Priority      uint32 `json:"priority,omitempty" yaml:"priority,omitempty"`
}

type OSPF6Redistribute struct {
	Connected bool `json:"connected,omitempty" yaml:"connected,omitempty"`
	Static    bool `json:"static,omitempty" yaml:"static,omitempty"`
	BGP       bool `json:"bgp,omitempty" yaml:"bgp,omitempty"`
}

type OSPF6DefaultInfo struct {
	Originate  bool   `json:"originate" yaml:"originate"`
	Always     bool   `json:"always,omitempty" yaml:"always,omitempty"`
	Metric     uint32 `json:"metric,omitempty" yaml:"metric,omitempty"`
	MetricType uint32 `json:"metric-type,omitempty" yaml:"metric-type,omitempty"`
}
