package ip

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
