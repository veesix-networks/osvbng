package protocols

type BGPSendCommunity string

const (
	SendCommunityNone     BGPSendCommunity = ""
	SendCommunityStandard BGPSendCommunity = "standard"
	SendCommunityExtended BGPSendCommunity = "extended"
	SendCommunityBoth     BGPSendCommunity = "both"
	SendCommunityAll      BGPSendCommunity = "all"
)

type BGPConfig struct {
	ASN         uint32                   `json:"asn" yaml:"asn"`
	RouterID    string                   `json:"router-id,omitempty" yaml:"router-id,omitempty"`
	PeerGroups  map[string]*BGPPeerGroup `json:"peer-groups,omitempty" yaml:"peer-groups,omitempty"`
	Neighbors   map[string]*BGPNeighbor  `json:"neighbors,omitempty" yaml:"neighbors,omitempty"`
	IPv4Unicast *BGPAddressFamily        `json:"ipv4-unicast,omitempty" yaml:"ipv4-unicast,omitempty"`
	IPv6Unicast *BGPAddressFamily        `json:"ipv6-unicast,omitempty" yaml:"ipv6-unicast,omitempty"`
	IPv4VPN     *BGPVPNAddressFamily     `json:"ipv4-vpn,omitempty" yaml:"ipv4-vpn,omitempty"`
	IPv6VPN     *BGPVPNAddressFamily     `json:"ipv6-vpn,omitempty" yaml:"ipv6-vpn,omitempty"`
	VRF         map[string]*BGPVRFConfig `json:"vrf,omitempty" yaml:"vrf,omitempty"`
}

type BGPPeerGroup struct {
	RemoteAS    uint32                `json:"remote-as,omitempty" yaml:"remote-as,omitempty"`
	IPv4Unicast *BGPNeighborAFIConfig `json:"ipv4-unicast,omitempty" yaml:"ipv4-unicast,omitempty"`
	IPv6Unicast *BGPNeighborAFIConfig `json:"ipv6-unicast,omitempty" yaml:"ipv6-unicast,omitempty"`
}

type BGPAddressFamily struct {
	Neighbors    map[string]*BGPNeighbor `json:"neighbors,omitempty" yaml:"neighbors,omitempty"`
	Networks     map[string]*BGPNetwork  `json:"networks,omitempty" yaml:"networks,omitempty"`
	Redistribute *BGPRedistribute        `json:"redistribute,omitempty" yaml:"redistribute,omitempty"`
}

type BGPNetwork struct {
	RouteMap string `json:"route-map,omitempty" yaml:"route-map,omitempty"`
}

type BGPRedistribute struct {
	Connected bool `json:"connected,omitempty" yaml:"connected,omitempty"`
	Static    bool `json:"static,omitempty" yaml:"static,omitempty"`
}

type BGPVRFConfig struct {
	RouterID    string          `json:"router-id,omitempty" yaml:"router-id,omitempty"`
	RD          string          `json:"rd,omitempty" yaml:"rd,omitempty"`
	IPv4Unicast *BGPVRFAFConfig `json:"ipv4-unicast,omitempty" yaml:"ipv4-unicast,omitempty"`
	IPv6Unicast *BGPVRFAFConfig `json:"ipv6-unicast,omitempty" yaml:"ipv6-unicast,omitempty"`
}

type BGPVPNAddressFamily struct {
	Neighbors map[string]*BGPNeighborAFIConfig `json:"neighbors,omitempty" yaml:"neighbors,omitempty"`
}

type BGPVRFAFConfig struct {
	Neighbors    map[string]*BGPNeighbor `json:"neighbors,omitempty" yaml:"neighbors,omitempty"`
	Networks     map[string]*BGPNetwork  `json:"networks,omitempty" yaml:"networks,omitempty"`
	Redistribute *BGPRedistribute        `json:"redistribute,omitempty" yaml:"redistribute,omitempty"`
	LabelVPN     string                  `json:"label-vpn,omitempty" yaml:"label-vpn,omitempty"`
	ExportVPN    bool                    `json:"export-vpn,omitempty" yaml:"export-vpn,omitempty"`
	ImportVPN    bool                    `json:"import-vpn,omitempty" yaml:"import-vpn,omitempty"`
}

type BGPNeighbor struct {
	Peer        string                `json:"peer,omitempty" yaml:"peer,omitempty"`
	PeerGroup   string                `json:"peer-group,omitempty" yaml:"peer-group,omitempty"`
	RemoteAS    uint32                `json:"remote-as,omitempty" yaml:"remote-as,omitempty"`
	BFD         bool                  `json:"bfd,omitempty" yaml:"bfd,omitempty"`
	Description string                `json:"description,omitempty" yaml:"description,omitempty"`
	IPv4Unicast *BGPNeighborAFIConfig `json:"ipv4-unicast,omitempty" yaml:"ipv4-unicast,omitempty"`
	IPv6Unicast *BGPNeighborAFIConfig `json:"ipv6-unicast,omitempty" yaml:"ipv6-unicast,omitempty"`
}

type BGPNeighborAFIConfig struct {
	NextHopSelf   bool             `json:"next-hop-self,omitempty" yaml:"next-hop-self,omitempty"`
	SendCommunity BGPSendCommunity `json:"send-community,omitempty" yaml:"send-community,omitempty"`
	RouteMapOut   string           `json:"route-map-out,omitempty" yaml:"route-map-out,omitempty"`
	RouteMapIn    string           `json:"route-map-in,omitempty" yaml:"route-map-in,omitempty"`
}
