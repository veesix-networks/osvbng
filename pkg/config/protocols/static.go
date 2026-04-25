package protocols

import "sort"

type StaticConfig struct {
	IPv4 []StaticRoute `json:"ipv4,omitempty" yaml:"ipv4,omitempty"`
	IPv6 []StaticRoute `json:"ipv6,omitempty" yaml:"ipv6,omitempty"`
}

type StaticRoute struct {
	Destination string `json:"destination" yaml:"destination"`
	NextHop     string `json:"next-hop,omitempty" yaml:"next-hop,omitempty"`
	Device      string `json:"device,omitempty" yaml:"device,omitempty"`
	VRF         string `json:"vrf,omitempty" yaml:"vrf,omitempty"`
}

func StaticVRFs(s *StaticConfig) []string {
	if s == nil {
		return nil
	}
	seen := map[string]struct{}{}
	for _, r := range s.IPv4 {
		if r.VRF != "" {
			seen[r.VRF] = struct{}{}
		}
	}
	for _, r := range s.IPv6 {
		if r.VRF != "" {
			seen[r.VRF] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
