package mpls

import (
	"github.com/veesix-networks/osvbng/pkg/config"
)

func collectMPLSInterfaces(cfg *config.Config) []string {
	seen := make(map[string]bool)

	if cfg.Protocols.OSPF != nil && cfg.Protocols.OSPF.Enabled {
		for _, area := range cfg.Protocols.OSPF.Areas {
			for iface := range area.Interfaces {
				seen[iface] = true
			}
		}
	}

	if cfg.Protocols.ISIS != nil && cfg.Protocols.ISIS.Enabled {
		for iface := range cfg.Protocols.ISIS.Interfaces {
			seen[iface] = true
		}
	}

	result := make([]string, 0, len(seen))
	for iface := range seen {
		result = append(result, iface)
	}
	return result
}
