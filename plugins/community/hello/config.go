package hello

import (
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/conf"
)

const Namespace = "example.hello"

type Config struct {
	Enabled bool   `json:"enabled" yaml:"enabled"`
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
}

func init() {
	conf.RegisterPluginConfig(Namespace, Config{})
	component.Register(Namespace, NewComponent,
		component.WithAuthor("veesix ::networks"),
		component.WithVersion("1.0.0"),
	)
}
