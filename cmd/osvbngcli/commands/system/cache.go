package system

import (
	"github.com/veesix-networks/osvbng/cmd/osvbngcli/commands"
	"github.com/veesix-networks/osvbng/pkg/cli"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

func init() {
	cli.RegisterRoot("core", &cli.RootCommand{
		Path:        []string{"show", "system", "cache"},
		Description: "Internal System config and state",
	})

	cli.Register("core", &cli.Command{
		Path:        []string{"show", "system", "cache", "statistics"},
		Description: "Display internal cache stats",
		Handler:     commands.ShowHandlerFunc(paths.SystemCacheStatistics),
	})

	cli.Register("core", &cli.Command{
		Path:        []string{"show", "system", "cache", "keys"},
		Description: "Display internal cache keys",
		Handler:     commands.ShowHandlerFunc(paths.SystemCacheKeys),
		Arguments: []*cli.Argument{
			{Name: "pattern", Description: "Cache Key Pattern (default: *)", Type: cli.ArgUserInput},
		},
	})

	cli.Register("core", &cli.Command{
		Path:        []string{"show", "system", "cache", "key"},
		Description: "Display internal cache key",
		Handler:     commands.ShowHandlerFunc(paths.SystemCacheKey),
		Arguments: []*cli.Argument{
			{Name: "key", Description: "Cache Key", Type: cli.ArgUserInput},
		},
	})
}
