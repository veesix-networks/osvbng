package protocols

type LDPConfig struct {
	Enabled             bool                          `json:"enabled" yaml:"enabled"`
	RouterID            string                        `json:"router-id,omitempty" yaml:"router-id,omitempty"`
	AddressFamilies     *LDPAddressFamilies           `json:"address-families,omitempty" yaml:"address-families,omitempty"`
	DiscoveryHelloHold  uint32                        `json:"discovery-hello-holdtime,omitempty" yaml:"discovery-hello-holdtime,omitempty"`
	DiscoveryHelloIntv  uint32                        `json:"discovery-hello-interval,omitempty" yaml:"discovery-hello-interval,omitempty"`
	OrderedControl      bool                          `json:"ordered-control,omitempty" yaml:"ordered-control,omitempty"`
	DualStackPreferIPv4 bool                          `json:"dual-stack-prefer-ipv4,omitempty" yaml:"dual-stack-prefer-ipv4,omitempty"`
	Neighbors           map[string]*LDPNeighborConfig `json:"neighbors,omitempty" yaml:"neighbors,omitempty"`
}

type LDPAddressFamilies struct {
	IPv4 *LDPAddressFamily `json:"ipv4,omitempty" yaml:"ipv4,omitempty"`
	IPv6 *LDPAddressFamily `json:"ipv6,omitempty" yaml:"ipv6,omitempty"`
}

type LDPAddressFamily struct {
	TransportAddress    string `json:"transport-address,omitempty" yaml:"transport-address,omitempty"`
	LabelLocalAdvertise string `json:"label-local-advertise,omitempty" yaml:"label-local-advertise,omitempty"`
}

type LDPNeighborConfig struct {
	Password string `json:"password,omitempty" yaml:"password,omitempty"`
	HoldTime uint32 `json:"session-holdtime,omitempty" yaml:"session-holdtime,omitempty"`
}
