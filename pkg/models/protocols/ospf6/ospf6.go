package ospf6

type Neighbor struct {
	NeighborID     string `json:"neighborId"`
	Priority       int    `json:"priority"`
	DeadTime       string `json:"deadTime"`
	State          string `json:"state"`
	IfState        string `json:"ifState"`
	Duration       string `json:"duration"`
	InterfaceName  string `json:"interfaceName"`
	InterfaceState string `json:"interfaceState"`
}
