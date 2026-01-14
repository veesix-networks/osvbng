package api

import (
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/configmgr"
)

const Namespace = "northbound.api"

type Config struct {
	Enabled       bool   `json:"enabled" yaml:"enabled"`
	ListenAddress string `json:"listen_address,omitempty" yaml:"listen_address,omitempty"`
}

func init() {
	configmgr.RegisterPluginConfig(Namespace, Config{})

	component.Register(Namespace, NewComponent,
		component.WithAuthor("Veesix Networks"),
		component.WithVersion("1.0.0"),
	)
}
