package protocols

type StaticConfig struct {
	IPv4 []StaticRoute `json:"ipv4,omitempty" yaml:"ipv4,omitempty"`
	IPv6 []StaticRoute `json:"ipv6,omitempty" yaml:"ipv6,omitempty"`
}

type StaticRoute struct {
	Destination string `json:"destination" yaml:"destination"`
	NextHop     string `json:"next-hop,omitempty" yaml:"next-hop,omitempty"`
	Device      string `json:"device,omitempty" yaml:"device,omitempty"`
}
