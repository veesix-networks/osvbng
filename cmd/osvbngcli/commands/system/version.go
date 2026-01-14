package system

import (
	"github.com/veesix-networks/osvbng/cmd/osvbngcli/commands"
	"github.com/veesix-networks/osvbng/pkg/cli"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

func init() {
	cli.Register("core", &cli.Command{
		Path:        []string{"show", "system", "version"},
		Description: "Display system version information",
		Handler:     commands.ShowHandlerFunc(paths.SystemVersion),
	})
}
