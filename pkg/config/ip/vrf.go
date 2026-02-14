package ip

type VRFSConfig struct {
	Description        string                 `json:"description,omitempty" yaml:"description,omitempty"`
	RouteDistinguisher string                 `json:"rd,omitempty" yaml:"rd,omitempty"`
	ImportRouteTargets []string               `json:"import-route-targets,omitempty" yaml:"import-route-targets,omitempty"`
	ExportRouteTargets []string               `json:"export-route-targets,omitempty" yaml:"export-route-targets,omitempty"`
	AddressFamilies    VRFAddressFamilyConfig `json:"address-families,omitempty" yaml:"address-families,omitempty"`
}

type VRFAddressFamilyConfig struct {
	IPv4Unicast *VRFAFConfig `json:"ipv4-unicast,omitempty" yaml:"ipv4-unicast,omitempty"`
	IPv6Unicast *VRFAFConfig `json:"ipv6-unicast,omitempty" yaml:"ipv6-unicast,omitempty"`
}

type VRFAFConfig struct {
	Enabled           bool   `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	ImportRoutePolicy string `json:"import-route-policy,omitempty" yaml:"import-route-policy,omitempty"`
	ExportRoutePolicy string `json:"export-route-policy,omitempty" yaml:"export-route-policy,omitempty"`
}
