package veesixtest

import (
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/configmgr"
)

const Namespace = "veesixtest"

type Config struct {
	Enabled bool   `json:"enabled" yaml:"enabled"`
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
}

func init() {
	// For production plugins, consider Enabled: false to require explicit configuration
	configmgr.RegisterPluginConfig(Namespace, Config{
		Enabled: true,
		Message: "Default message",
	})
	component.Register(Namespace, NewComponent,
		component.WithAuthor("veesix ::networks"),
		component.WithVersion("1.0.0"),
	)
}
