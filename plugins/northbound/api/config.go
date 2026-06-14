package api

import (
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/configmgr"
	"github.com/veesix-networks/osvbng/pkg/netbind"
)

const Namespace = "northbound.api"

type Config struct {
	netbind.ListenerBinding `json:",inline" yaml:",inline"`
	Enabled                 bool                    `json:"enabled" yaml:"enabled"`
	ListenAddress           string                  `json:"listen_address,omitempty" yaml:"listen_address,omitempty"`
	TLS                     netbind.ServerTLSConfig `json:"tls,omitempty" yaml:"tls,omitempty"`
	UDS                     UDSConfig               `json:"uds,omitempty" yaml:"uds,omitempty"`
}

type UDSConfig struct {
	Enabled bool   `json:"enabled" yaml:"enabled"`
	Path    string `json:"path,omitempty" yaml:"path,omitempty"`
	Mode    string `json:"mode,omitempty" yaml:"mode,omitempty"`
	Group   string `json:"group,omitempty" yaml:"group,omitempty"`
}

func init() {
	configmgr.RegisterPluginConfig(Namespace, Config{})

	component.Register(Namespace, NewComponent,
		component.WithAuthor("Veesix Networks"),
		component.WithVersion("1.0.0"),
	)
}
