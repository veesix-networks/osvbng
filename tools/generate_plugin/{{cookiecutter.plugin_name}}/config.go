package {{cookiecutter.plugin_name}}

import (
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/configmgr"
)

const Namespace = "{{cookiecutter.plugin_namespace}}"

type Config struct {
	Enabled bool   `json:"enabled" yaml:"enabled"`
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
}

func init() {
	configmgr.RegisterPluginConfig(Namespace, Config{})
	component.Register(Namespace, NewComponent,
		component.WithAuthor("{{cookiecutter.author_name}}"),
		component.WithVersion("{{cookiecutter.version}}"),
	)
}
