package veesixtest

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/cmd/osvbngcli/commands"
	"github.com/veesix-networks/osvbng/pkg/cli"
)

func init() {
	cli.RegisterRoot(Namespace, &cli.RootCommand{
		Path:        []string{"show", "veesixtest"},
		Description: "A new osvbng plugin",
	})

	cli.Register(Namespace, &cli.Command{
		Path:        []string{"show", "veesixtest", "status"},
		Description: "Display veesixtest status",
		Handler:     cmdShowStatus,
	})

	cli.RegisterRoot(Namespace, &cli.RootCommand{
		Path:        []string{"set", "veesixtest"},
		Description: "Configure veesixtest plugin",
	})

	cli.Register(Namespace, &cli.Command{
		Path:        []string{"set", "veesixtest", "message"},
		Description: "Set the plugin message",
		Handler:     cmdSetMessage,
		Arguments: []*cli.Argument{
			{Name: "text", Description: "Message text", Type: cli.ArgUserInput},
		},
	})
}

func cmdShowStatus(ctx context.Context, c interface{}, args []string) error {
	return commands.ExecuteShowHandler(ctx, c, ShowStatusPath, args)
}

func cmdSetMessage(ctx context.Context, c interface{}, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: set veesixtest message <text>")
	}

	message := args[0]
	return commands.ExecuteConfigSet(ctx, c, ConfMessagePath, message)
}
