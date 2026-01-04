package hello

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/cmd/osvbngcli/commands"
	"github.com/veesix-networks/osvbng/pkg/cli"
)

func init() {
	cli.RegisterRoot(Namespace, &cli.RootCommand{
		Path:        []string{"show", "example", "hello"},
		Description: "Example hello plugin",
	})

	cli.Register(Namespace, &cli.Command{
		Path:        []string{"show", "example", "hello", "status"},
		Description: "Display example hello status",
		Handler:     cmdShowHelloStatus,
	})

	cli.RegisterRoot(Namespace, &cli.RootCommand{
		Path:        []string{"set", "example", "hello"},
		Description: "Configure example hello plugin",
	})

	cli.Register(Namespace, &cli.Command{
		Path:        []string{"set", "example", "hello", "message"},
		Description: "Set the hello message",
		Handler:     cmdSetHelloMessage,
		Arguments: []*cli.Argument{
			{Name: "text", Description: "Message text", Type: cli.ArgUserInput},
		},
	})
}

func cmdShowHelloStatus(ctx context.Context, c interface{}, args []string) error {
	return commands.ExecuteShowHandler(ctx, c, ShowStatusPath, args)
}

func cmdSetHelloMessage(ctx context.Context, c interface{}, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: set example hello message <text>")
	}

	message := args[0]
	return commands.ExecuteConfigSet(ctx, c, ConfMessagePath, message)
}
