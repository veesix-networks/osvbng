package protocols

type ProtocolConfig struct {
	BGP    *BGPConfig    `json:"bgp,omitempty" yaml:"bgp,omitempty"`
	OSPF   *OSPFConfig   `json:"ospf,omitempty" yaml:"ospf,omitempty"`
	Static *StaticConfig `json:"static,omitempty" yaml:"static,omitempty"`
}
