package subscriber

import (
	"github.com/veesix-networks/osvbng/cmd/osvbngcli/commands"
	"github.com/veesix-networks/osvbng/pkg/cli"
	"github.com/veesix-networks/osvbng/pkg/show/paths"
)

func init() {
	cli.RegisterRoot("core", &cli.RootCommand{
		Path:        []string{"show", "subscriber"},
		Description: "Subscriber information",
	})

	cli.Register("core", &cli.Command{
		Path:        []string{"show", "subscriber", "sessions"},
		Description: "Display subscriber sessions",
		Handler:     commands.ShowHandlerFunc(paths.SubscriberSessions),
	})

	cli.Register("core", &cli.Command{
		Path:        []string{"show", "subscriber", "session"},
		Description: "Display subscriber session details",
		Handler:     commands.ShowHandlerFunc(paths.SubscriberSession),
		Arguments: []*cli.Argument{
			{Name: "session-id", Description: "Session identifier", Type: cli.ArgUserInput},
		},
	})

	cli.Register("core", &cli.Command{
		Path:        []string{"show", "subscriber", "stats"},
		Description: "Display subscriber statistics",
		Handler:     commands.ShowHandlerFunc(paths.SubscriberStats),
	})
}
