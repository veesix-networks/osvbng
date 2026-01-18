package config

import (
	"github.com/veesix-networks/osvbng/pkg/config/aaa"
	"github.com/veesix-networks/osvbng/pkg/config/interfaces"
	"github.com/veesix-networks/osvbng/pkg/config/ip"
	"github.com/veesix-networks/osvbng/pkg/config/protocols"
	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
	"github.com/veesix-networks/osvbng/pkg/config/system"
)

type Config struct {
	Logging         system.LoggingConfig               `json:"logging,omitempty" yaml:"logging,omitempty"`
	Dataplane       system.DataplaneConfig             `json:"dataplane,omitempty" yaml:"dataplane,omitempty"`
	SubscriberGroup subscriber.SubscriberGroupConfig   `json:"subscriber_group,omitempty" yaml:"subscriber_group,omitempty"`
	DHCP            ip.DHCPConfig                      `json:"dhcp,omitempty" yaml:"dhcp,omitempty"`
	Monitoring      system.MonitoringConfig            `json:"monitoring,omitempty" yaml:"monitoring,omitempty"`
	API             system.APIConfig                   `json:"api,omitempty" yaml:"api,omitempty"`
	Interfaces      map[string]*interfaces.InterfaceConfig `json:"interfaces,omitempty" yaml:"interfaces,omitempty"`
	Protocols       protocols.ProtocolConfig           `json:"protocols,omitempty" yaml:"protocols,omitempty"`
	AAA             aaa.AAAConfig                      `json:"aaa,omitempty" yaml:"aaa,omitempty"`
	VRFS            []ip.VRFSConfig                    `json:"vrfs,omitempty" yaml:"vrfs,omitempty"`
	Plugins         map[string]interface{}             `json:"plugins,omitempty" yaml:"plugins,omitempty"`
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
