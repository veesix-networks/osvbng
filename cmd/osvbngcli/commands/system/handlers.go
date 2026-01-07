package system

import (
	"github.com/veesix-networks/osvbng/cmd/osvbngcli/commands"
	"github.com/veesix-networks/osvbng/pkg/cli"
	"github.com/veesix-networks/osvbng/pkg/show/paths"
)

func init() {
	cli.Register("core", &cli.Command{
		Path:        []string{"show", "system", "handlers", "conf"},
		Description: "Display registered configuration handlers",
		Handler:     commands.ShowHandlerFunc(paths.SystemConfHandlers),
	})

	cli.Register("core", &cli.Command{
		Path:        []string{"show", "system", "handlers", "show"},
		Description: "Display registered show handlers",
		Handler:     commands.ShowHandlerFunc(paths.SystemShowHandlers),
	})

	cli.Register("core", &cli.Command{
		Path:        []string{"show", "system", "handlers", "oper"},
		Description: "Display registered oper handlers",
		Handler:     commands.ShowHandlerFunc(paths.SystemOperHandlers),
	})
}
