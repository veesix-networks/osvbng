package config

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	bngpb "github.com/veesix-networks/osvbng/api/proto"
	"github.com/veesix-networks/osvbng/pkg/cli"
)

func init() {
	cli.RegisterRoot("core", &cli.RootCommand{
		Path:        []string{"show", "config"},
		Description: "Configuration information",
	})

	cli.Register("core", &cli.Command{
		Path:        []string{"show", "running-config"},
		Description: "Display running configuration",
		Handler:     cmdShowRunningConfig,
	})

	cli.Register("core", &cli.Command{
		Path:        []string{"show", "startup-config"},
		Description: "Display startup configuration",
		Handler:     cmdShowStartupConfig,
	})

	cli.Register("core", &cli.Command{
		Path:        []string{"show", "config", "history"},
		Description: "Display configuration version history",
		Handler:     cmdShowConfigHistory,
	})

	cli.Register("core", &cli.Command{
		Path:        []string{"show", "config", "version"},
		Description: "Display specific configuration version",
		Handler:     cmdShowConfigVersion,
		Arguments: []*cli.Argument{
			{Name: "version", Description: "Version number", Type: cli.ArgUserInput},
		},
	})

	cli.Register("core", &cli.Command{
		Path:        []string{"configure"},
		Description: "Enter configuration mode",
		Handler:     cmdConfigure,
	})

	cli.Register("core", &cli.Command{
		Path:        []string{"commit"},
		Description: "Commit configuration changes",
		Handler:     cmdCommit,
		Arguments: []*cli.Argument{
			{Name: "message", Description: "Commit message (optional)", Type: cli.ArgUserInput},
		},
	})

	cli.Register("core", &cli.Command{
		Path:        []string{"discard"},
		Description: "Discard configuration changes",
		Handler:     cmdDiscard,
	})
}

type cliContext interface {
	GetClient() bngpb.BNGServiceClient
	GetConfigMode() bool
	GetConfigSessionID() string
}

func cmdShowRunningConfig(ctx context.Context, c interface{}, args []string) error {
	wrapper, ok := c.(cliContext)
	if !ok {
		return fmt.Errorf("invalid CLI context")
	}

	client := wrapper.GetClient()
	resp, err := client.GetRunningConfig(ctx, &bngpb.GetRunningConfigRequest{})
	if err != nil {
		return fmt.Errorf("failed to get running config: %w", err)
	}

	fmt.Println(resp.ConfigYaml)
	return nil
}

func cmdShowStartupConfig(ctx context.Context, c interface{}, args []string) error {
	wrapper, ok := c.(cliContext)
	if !ok {
		return fmt.Errorf("invalid CLI context")
	}

	client := wrapper.GetClient()
	resp, err := client.GetStartupConfig(ctx, &bngpb.GetStartupConfigRequest{})
	if err != nil {
		return fmt.Errorf("failed to get startup config: %w", err)
	}

	fmt.Println(resp.ConfigYaml)
	return nil
}

func cmdShowConfigHistory(ctx context.Context, c interface{}, args []string) error {
	wrapper, ok := c.(cliContext)
	if !ok {
		return fmt.Errorf("invalid CLI context")
	}

	client := wrapper.GetClient()
	resp, err := client.ListVersions(ctx, &bngpb.ListVersionsRequest{})
	if err != nil {
		return fmt.Errorf("failed to list versions: %w", err)
	}

	if len(resp.Versions) == 0 {
		fmt.Println("No configuration versions")
		return nil
	}

	fmt.Println("Configuration version history:")
	for _, v := range resp.Versions {
		fmt.Printf("  Version %d - %s\n", v.Version, v.CommitMsg)
	}

	return nil
}

func cmdShowConfigVersion(ctx context.Context, c interface{}, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: show config version <version-number>")
	}

	wrapper, ok := c.(cliContext)
	if !ok {
		return fmt.Errorf("invalid CLI context")
	}

	version, err := strconv.ParseInt(args[0], 10, 32)
	if err != nil {
		return fmt.Errorf("invalid version number: %w", err)
	}

	client := wrapper.GetClient()
	resp, err := client.GetVersion(ctx, &bngpb.GetVersionRequest{
		Version: int32(version),
	})
	if err != nil {
		return fmt.Errorf("failed to get version: %w", err)
	}

	fmt.Printf("Version %d\n", resp.Version.Version)
	fmt.Printf("Commit message: %s\n", resp.Version.CommitMsg)
	fmt.Println("\nChanges:")
	for _, change := range resp.Version.Changes {
		fmt.Printf("  %s: %s = %s\n", change.Type, change.Path, change.Value)
	}

	return nil
}

func cmdConfigure(ctx context.Context, c interface{}, args []string) error {
	wrapper, ok := c.(interface {
		GetClient() bngpb.BNGServiceClient
		GetConfigMode() bool
	})
	if !ok {
		return fmt.Errorf("invalid CLI context")
	}

	if wrapper.GetConfigMode() {
		return fmt.Errorf("already in configuration mode")
	}

	client := wrapper.GetClient()
	resp, err := client.ConfigEnter(ctx, &bngpb.ConfigEnterRequest{})
	if err != nil {
		return fmt.Errorf("failed to enter config mode: %w", err)
	}

	// We need to update the CLI's configMode and configSessionID
	// This is a bit hacky but necessary
	type configSetter interface {
		SetConfigMode(bool)
		SetConfigSessionID(string)
	}

	if setter, ok := c.(configSetter); ok {
		setter.SetConfigMode(true)
		setter.SetConfigSessionID(resp.SessionId)
	}

	fmt.Println("Entered configuration mode")
	fmt.Println("Use 'commit' to apply changes, 'discard' to cancel, 'exit' to leave config mode")
	return nil
}

func cmdCommit(ctx context.Context, c interface{}, args []string) error {
	wrapper, ok := c.(cliContext)
	if !ok {
		return fmt.Errorf("invalid CLI context")
	}

	if !wrapper.GetConfigMode() {
		return fmt.Errorf("not in configuration mode")
	}

	commitMsg := "Configuration change"
	if len(args) > 0 {
		commitMsg = strings.Join(args, " ")
	}

	client := wrapper.GetClient()
	resp, err := client.ConfigCommit(ctx, &bngpb.ConfigCommitRequest{
		SessionId: wrapper.GetConfigSessionID(),
		CommitMsg: commitMsg,
	})
	if err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("%s", resp.Message)
	}

	// Exit config mode
	if setter, ok := c.(interface {
		SetConfigMode(bool)
		SetConfigSessionID(string)
	}); ok {
		setter.SetConfigMode(false)
		setter.SetConfigSessionID("")
	}

	fmt.Printf("Configuration committed (version %d)\n", resp.Version)
	return nil
}

func cmdDiscard(ctx context.Context, c interface{}, args []string) error {
	wrapper, ok := c.(cliContext)
	if !ok {
		return fmt.Errorf("invalid CLI context")
	}

	if !wrapper.GetConfigMode() {
		return fmt.Errorf("not in configuration mode")
	}

	client := wrapper.GetClient()
	_, err := client.ConfigDiscard(ctx, &bngpb.ConfigDiscardRequest{
		SessionId: wrapper.GetConfigSessionID(),
	})
	if err != nil {
		return fmt.Errorf("failed to discard changes: %w", err)
	}

	// Exit config mode
	if setter, ok := c.(interface {
		SetConfigMode(bool)
		SetConfigSessionID(string)
	}); ok {
		setter.SetConfigMode(false)
		setter.SetConfigSessionID("")
	}

	fmt.Println("Configuration changes discarded")
	return nil
}
