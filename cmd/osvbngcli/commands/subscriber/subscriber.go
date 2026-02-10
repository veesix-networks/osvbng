package subscriber

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/veesix-networks/osvbng/cmd/osvbngcli/commands"
	"github.com/veesix-networks/osvbng/pkg/cli"
	operpaths "github.com/veesix-networks/osvbng/pkg/handlers/oper/paths"
	showpaths "github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

func init() {
	cli.RegisterRoot("core", &cli.RootCommand{
		Path:        []string{"show", "subscriber"},
		Description: "Subscriber information",
	})

	cli.Register("core", &cli.Command{
		Path:        []string{"show", "subscriber", "sessions"},
		Description: "Display subscriber sessions",
		Handler:     commands.ShowHandlerFunc(showpaths.SubscriberSessions),
	})

	cli.Register("core", &cli.Command{
		Path:        []string{"show", "subscriber", "session"},
		Description: "Display subscriber session details",
		Handler:     commands.ShowHandlerFunc(showpaths.SubscriberSession),
		Arguments: []*cli.Argument{
			{Name: "session-id", Description: "Session identifier", Type: cli.ArgUserInput},
		},
	})

	cli.Register("core", &cli.Command{
		Path:        []string{"show", "subscriber", "stats"},
		Description: "Display subscriber statistics",
		Handler:     commands.ShowHandlerFunc(showpaths.SubscriberStats),
	})

	cli.RegisterRoot("core", &cli.RootCommand{
		Path:        []string{"exec", "subscriber", "session", "clear"},
		Description: "Clear subscriber sessions",
	})

	cli.Register("core", &cli.Command{
		Path:        []string{"exec", "subscriber", "session", "clear", "session-id"},
		Description: "Clear session by session ID",
		Handler:     clearByField("session_id"),
		Arguments: []*cli.Argument{
			{Name: "id", Description: "Session identifier", Type: cli.ArgUserInput},
		},
	})

	cli.Register("core", &cli.Command{
		Path:        []string{"exec", "subscriber", "session", "clear", "mac"},
		Description: "Clear sessions by MAC address",
		Handler:     clearByField("mac"),
		Arguments: []*cli.Argument{
			{Name: "address", Description: "MAC address (e.g. 00:11:22:33:44:55)", Type: cli.ArgUserInput},
		},
	})

	cli.Register("core", &cli.Command{
		Path:        []string{"exec", "subscriber", "session", "clear", "ipv4"},
		Description: "Clear sessions by IPv4 address",
		Handler:     clearByField("ipv4"),
		Arguments: []*cli.Argument{
			{Name: "address", Description: "IPv4 address", Type: cli.ArgUserInput},
		},
	})

	cli.Register("core", &cli.Command{
		Path:        []string{"exec", "subscriber", "session", "clear", "ipv6"},
		Description: "Clear sessions by IPv6 address",
		Handler:     clearByField("ipv6"),
		Arguments: []*cli.Argument{
			{Name: "address", Description: "IPv6 address", Type: cli.ArgUserInput},
		},
	})

	cli.Register("core", &cli.Command{
		Path:        []string{"exec", "subscriber", "session", "clear", "username"},
		Description: "Clear sessions by username",
		Handler:     clearByField("username"),
		Arguments: []*cli.Argument{
			{Name: "name", Description: "Username", Type: cli.ArgUserInput},
		},
	})
}

func clearByField(field string) cli.CommandHandler {
	return func(ctx context.Context, c interface{}, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("%s value required", field)
		}

		body, err := json.Marshal(map[string]string{field: args[0]})
		if err != nil {
			return fmt.Errorf("failed to encode request: %w", err)
		}

		return commands.ExecuteOper(ctx, c, operpaths.SubscriberSessionClear, string(body))
	}
}
