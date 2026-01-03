package cli

import (
	"context"

	"github.com/veesix-networks/osvbng/cmd/osvbngcli/commands"
	"github.com/veesix-networks/osvbng/pkg/cli"
	"github.com/veesix-networks/osvbng/pkg/show/paths"
)

func init() {
	cli.RegisterRoot("exporter.prometheus", &cli.RootCommand{
		Path:        []string{"show", "exporters", "prometheus"},
		Description: "Prometheus exporter",
	})

	cli.Register("exporter.prometheus", &cli.Command{
		Path:        []string{"show", "exporters", "prometheus", "status"},
		Description: "Display Prometheus exporter status",
		Handler:     cmdShowPrometheusStatus,
	})
}

func cmdShowPrometheusStatus(ctx context.Context, c interface{}, args []string) error {
	return commands.ExecuteShowHandler(ctx, c, paths.Path("exporters.prometheus.status"), args)
}
