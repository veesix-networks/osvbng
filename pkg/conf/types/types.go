package types

import (
	"time"
)

type SessionID string

type ValidationError struct {
	Path     string
	Code     string
	Message  string
	Severity string
}

type ConfigVersion struct {
	Version   int
	Timestamp time.Time
	Config    *Config
	Changes   []Change
	CommitMsg string
}

type Change struct {
	Type  string
	Path  string
	Value interface{}
}

type Config struct {
	Interfaces map[string]*InterfaceConfig `json:"interfaces,omitempty" yaml:"interfaces,omitempty"`
	Protocols  *ProtocolConfig             `json:"protocols,omitempty" yaml:"protocols,omitempty"`
	AAA        *AAAConfig                  `json:"aaa,omitempty" yaml:"aaa,omitempty"`
	VRFS       []VRFSConfig                `json:"vrfs,omitempty" yaml:"vrfs,omitempty"`
}

type VRFSConfig struct {
	Name               string                 `json:"name" yaml:"name"`
	Description        string                 `json:"description,omitempty" yaml:"description,omitempty"`
	RouteDistinguisher string                 `json:"rd,omitempty" yaml:"rd,omitempty"`
	ImportRouteTargets []string               `json:"import-route-targets,omitempty" yaml:"import-route-targets,omitempty"`
	ExportRouteTargets []string               `json:"export-route-targets,omitempty" yaml:"export-route-targets,omitempty"`
	AddressFamilies    VRFAddressFamilyConfig `json:"address-families,omitempty" yaml:"address-families,omitempty"`
}

type VRFAddressFamilyConfig struct {
	IPv4Unicast *IPv4UnicastConfig `json:"ipv4-unicast,omitempty" yaml:"ipv4-unicast,omitempty"`
	IPv6Unicast *IPv6UnicastConfig `json:"ipv6-unicast,omitempty" yaml:"ipv6-unicast,omitempty"`
}

type IPv4UnicastConfig struct {
	ImportRoutePolicy string `json:"import-route-policy,omitempty" yaml:"import-route-policy,omitempty"`
	ExportRoutePolicy string `json:"export-route-policy,omitempty" yaml:"export-route-policy,omitempty"`
}

type IPv6UnicastConfig struct {
	ImportRoutePolicy string `json:"import-route-policy,omitempty" yaml:"import-route-policy,omitempty"`
	ExportRoutePolicy string `json:"export-route-policy,omitempty" yaml:"export-route-policy,omitempty"`
}

type InterfaceConfig struct {
	Name        string         `json:"name" yaml:"name"`
	Description string         `json:"description,omitempty" yaml:"description,omitempty"`
	Enabled     bool           `json:"enabled" yaml:"enabled"`
	MTU         int            `json:"mtu,omitempty" yaml:"mtu,omitempty"`
	Address     *AddressConfig `json:"address,omitempty" yaml:"address,omitempty"`

	Type   string      `json:"type,omitempty" yaml:"type,omitempty"`
	Parent string      `json:"parent,omitempty" yaml:"parent,omitempty"`
	VLANID int         `json:"vlan-id,omitempty" yaml:"vlan-id,omitempty"`
	Bond   *BondConfig `json:"bond,omitempty" yaml:"bond,omitempty"`
	LCP    bool        `json:"lcp,omitempty" yaml:"lcp,omitempty"`
}

type AddressConfig struct {
	IPv4 []string `json:"ipv4,omitempty" yaml:"ipv4,omitempty"`
	IPv6 []string `json:"ipv6,omitempty" yaml:"ipv6,omitempty"`
}

type BondConfig struct {
	Mode    string   `json:"mode" yaml:"mode"`
	Members []string `json:"members" yaml:"members"`
	MIIMon  int      `json:"miimon,omitempty" yaml:"miimon,omitempty"`
}

type ProtocolConfig struct {
	BGP    *BGPConfig    `json:"bgp,omitempty" yaml:"bgp,omitempty"`
	OSPF   *OSPFConfig   `json:"ospf,omitempty" yaml:"ospf,omitempty"`
	Static *StaticConfig `json:"static,omitempty" yaml:"static,omitempty"`
}

type StaticConfig struct {
	IPv4 []StaticRoute `json:"ipv4,omitempty" yaml:"ipv4,omitempty"`
	IPv6 []StaticRoute `json:"ipv6,omitempty" yaml:"ipv6,omitempty"`
}

type BGPConfig struct {
	ASN         uint32                   `json:"asn" yaml:"asn"`
	RouterID    string                   `json:"router-id,omitempty" yaml:"router-id,omitempty"`
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
	RemoteAS    uint32 `json:"remote-as" yaml:"remote-as"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

type OSPFConfig struct {
	RouterID string        `json:"router-id" yaml:"router-id"`
	Networks []OSPFNetwork `json:"networks,omitempty" yaml:"networks,omitempty"`
}

type OSPFNetwork struct {
	Prefix string `json:"prefix" yaml:"prefix"`
	Area   string `json:"area" yaml:"area"`
}

type StaticRoute struct {
	Destination string `json:"destination" yaml:"destination"`
	NextHop     string `json:"next-hop,omitempty" yaml:"next-hop,omitempty"`
	Device      string `json:"device,omitempty" yaml:"device,omitempty"`
}

type DiffResult struct {
	Added    []ConfigLine
	Deleted  []ConfigLine
	Modified []ConfigLine
}

type ConfigLine struct {
	Path  string
	Value string
}

type AAAConfig struct {
	NASIdentifier string `json:"nas_identifier,omitempty" yaml:"nas_identifier,omitempty"`
	NASIP         string `json:"nas_ip,omitempty" yaml:"nas_ip,omitempty"`
}

type Command struct {
	Type string
	Func func() error
}
