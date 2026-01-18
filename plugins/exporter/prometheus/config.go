package prometheus

import "github.com/veesix-networks/osvbng/pkg/configmgr"

const Namespace = "exporter.prometheus"

type Config struct {
	Enabled       bool   `yaml:"enabled" json:"enabled"`
	ListenAddress string `yaml:"listen_address" json:"listen_address"`
}

func init() {
	configmgr.RegisterPluginConfig(Namespace, Config{})
}
