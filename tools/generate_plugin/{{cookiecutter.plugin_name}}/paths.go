package {{cookiecutter.plugin_name}}

import (
	confpaths "github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	showpaths "github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	statepaths "github.com/veesix-networks/osvbng/pkg/state/paths"
)

const (
	ShowStatusPath  = showpaths.Path("{{cookiecutter.plugin_namespace}}.status")
	StateStatusPath = statepaths.Path("{{cookiecutter.plugin_namespace}}.status")
	ConfMessagePath = confpaths.Path("plugins.{{cookiecutter.plugin_namespace}}.message")
)
