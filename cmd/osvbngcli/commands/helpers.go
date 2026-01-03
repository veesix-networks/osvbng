package commands

import (
	"context"
	"encoding/json"
	"fmt"

	bngpb "github.com/veesix-networks/osvbng/api/proto"
	confpaths "github.com/veesix-networks/osvbng/pkg/conf/paths"
	showpaths "github.com/veesix-networks/osvbng/pkg/show/paths"
)

type CLIWrapper interface {
	GetClient() bngpb.BNGServiceClient
	GetConfigMode() bool
	GetConfigSessionID() string
	FormatOutput(data interface{}, format string) (string, error)
}

func ExecuteShowHandler(ctx context.Context, c interface{}, path showpaths.Path, args []string) error {
	wrapper, ok := c.(CLIWrapper)
	if !ok {
		return fmt.Errorf("invalid CLI context")
	}

	client := wrapper.GetClient()

	req := &bngpb.GetOperationalStatsRequest{
		Path: path.String(),
	}

	resp, err := client.GetOperationalStats(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to get stats: %w", err)
	}

	format := "cli"
	for i := 0; i < len(args); i++ {
		if args[i] == "|" && i+1 < len(args) {
			format = args[i+1]
			break
		}
	}

	rawData := resp.Metrics[path.String()]
	var data interface{}
	if err := json.Unmarshal(rawData, &data); err != nil {
		return fmt.Errorf("unmarshal data: %w", err)
	}

	output, err := wrapper.FormatOutput(data, format)
	if err != nil {
		return fmt.Errorf("format output: %w", err)
	}

	fmt.Print(output)
	return nil
}

func ExecuteConfigSet(ctx context.Context, c interface{}, path confpaths.Path, value string) error {
	wrapper, ok := c.(CLIWrapper)
	if !ok {
		return fmt.Errorf("invalid CLI context")
	}

	if !wrapper.GetConfigMode() {
		return fmt.Errorf("not in configuration mode. Use 'configure' first")
	}

	client := wrapper.GetClient()

	resp, err := client.ConfigSet(ctx, &bngpb.ConfigSetRequest{
		SessionId: wrapper.GetConfigSessionID(),
		Path:      path.String(),
		Value:     value,
	})
	if err != nil {
		return fmt.Errorf("failed to set config: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("%s", resp.Message)
	}

	fmt.Printf("Set %s = %s\n", path, value)
	return nil
}
