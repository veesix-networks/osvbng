package subscriber

import (
	"context"
	"fmt"

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
		Handler:     cmdShowSubscriberSessions,
	})

	cli.Register("core", &cli.Command{
		Path:        []string{"show", "subscriber", "session"},
		Description: "Display subscriber session details",
		Handler:     cmdShowSubscriberSession,
		Arguments: []*cli.Argument{
			{Name: "session-id", Description: "Session identifier", Type: cli.ArgUserInput},
		},
	})

	cli.Register("core", &cli.Command{
		Path:        []string{"show", "subscriber", "stats"},
		Description: "Display subscriber statistics",
		Handler:     cmdShowSubscriberStats,
	})
}

func cmdShowSubscriberSessions(ctx context.Context, c interface{}, args []string) error {
	return commands.ExecuteShowHandler(ctx, c, paths.SubscriberSessions, args)
}

func cmdShowSubscriberSession(ctx context.Context, c interface{}, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("session-id required")
	}
	return commands.ExecuteShowHandler(ctx, c, paths.SubscriberSession, args)
}

func cmdShowSubscriberStats(ctx context.Context, c interface{}, args []string) error {
	return commands.ExecuteShowHandler(ctx, c, paths.SubscriberStats, args)
}
