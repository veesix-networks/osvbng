package protocols

type ProtocolConfig struct {
	BGP    *BGPConfig    `json:"bgp,omitempty" yaml:"bgp,omitempty"`
	OSPF   *OSPFConfig   `json:"ospf,omitempty" yaml:"ospf,omitempty"`
	OSPF6  *OSPF6Config  `json:"ospf6,omitempty" yaml:"ospf6,omitempty"`
	ISIS   *ISISConfig   `json:"isis,omitempty" yaml:"isis,omitempty"`
	Static *StaticConfig `json:"static,omitempty" yaml:"static,omitempty"`
}
