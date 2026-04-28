package prometheus

import (
	"github.com/veesix-networks/osvbng/pkg/configmgr"
	"github.com/veesix-networks/osvbng/pkg/netbind"
)

const Namespace = "exporter.prometheus"

type Config struct {
	netbind.ListenerBinding `json:",inline" yaml:",inline"`
	Enabled                 bool                    `yaml:"enabled" json:"enabled"`
	ListenAddress           string                  `yaml:"listen_address" json:"listen_address"`
	TLS                     netbind.ServerTLSConfig `json:"tls,omitempty" yaml:"tls,omitempty"`
}

func init() {
	configmgr.RegisterPluginConfig(Namespace, Config{})
}
