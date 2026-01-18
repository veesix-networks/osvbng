package protocols

type BGPConfig struct {
	ASN         uint32                   `json:"asn" yaml:"asn"`
	RouterID    string                   `json:"router-id,omitempty" yaml:"router-id,omitempty"`
	Neighbors   map[string]*BGPNeighbor  `json:"neighbors,omitempty" yaml:"neighbors,omitempty"`
	IPv4Unicast *BGPAddressFamily        `json:"ipv4-unicast,omitempty" yaml:"ipv4-unicast,omitempty"`
	IPv6Unicast *BGPAddressFamily        `json:"ipv6-unicast,omitempty" yaml:"ipv6-unicast,omitempty"`
	VRF         map[string]*BGPVRFConfig `json:"vrf,omitempty" yaml:"vrf,omitempty"`
}

type BGPAddressFamily struct {
	Neighbors map[string]*BGPNeighbor `json:"neighbors,omitempty" yaml:"neighbors,omitempty"`
	Networks  []string                `json:"networks,omitempty" yaml:"networks,omitempty"`
}

type BGPVRFConfig struct {
	RouterID    string            `json:"router-id,omitempty" yaml:"router-id,omitempty"`
	RD          string            `json:"rd,omitempty" yaml:"rd,omitempty"`
	IPv4Unicast *BGPAddressFamily `json:"ipv4-unicast,omitempty" yaml:"ipv4-unicast,omitempty"`
	IPv6Unicast *BGPAddressFamily `json:"ipv6-unicast,omitempty" yaml:"ipv6-unicast,omitempty"`
}

type BGPNeighbor struct {
	Peer        string `json:"peer,omitempty" yaml:"peer,omitempty"`
	RemoteAS    uint32 `json:"remote_as,omitempty" yaml:"remote_as,omitempty"`
	BFD         bool   `json:"bfd,omitempty" yaml:"bfd,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}
