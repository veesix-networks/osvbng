package plugins

import (
	"github.com/veesix-networks/osvbng/cmd/osvbngcli/commands"
	"github.com/veesix-networks/osvbng/pkg/cli"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

func init() {
	cli.RegisterRoot("core", &cli.RootCommand{
		Path:        []string{"show", "plugins"},
		Description: "Component and Provider plugins",
	})

	cli.Register("core", &cli.Command{
		Path:        []string{"show", "plugins", "info"},
		Description: "Display all plugins and their metadata",
		Handler:     commands.ShowHandlerFunc(paths.PluginsInfo),
	})
}
