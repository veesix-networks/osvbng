package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	bngpb "github.com/veesix-networks/osvbng/api/proto"
	"github.com/veesix-networks/osvbng/pkg/show/paths"
)

func cmdShowSessions(ctx context.Context, cli *CLI, args []string) error {
	req := &bngpb.GetSessionsRequest{}

	for i := 0; i < len(args); i += 2 {
		if i+1 >= len(args) {
			return fmt.Errorf("missing value for %s", args[i])
		}

		switch args[i] {
		case "svlan":
			var svlan uint32
			if _, err := fmt.Sscanf(args[i+1], "%d", &svlan); err != nil {
				return fmt.Errorf("invalid svlan value: %s", args[i+1])
			}
			req.Svlan = svlan
		case "protocol":
			req.Protocol = args[i+1]
		case "access-type":
			req.AccessType = args[i+1]
		default:
			return fmt.Errorf("unknown filter: %s", args[i])
		}
	}

	resp, err := cli.client.GetSessions(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to get sessions: %w", err)
	}

	if len(resp.Sessions) == 0 {
		fmt.Println("No sessions found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SESSION-ID\tMAC\tVLAN\tSTATE\tPROTOCOL\tIP\tHOSTNAME")

	for _, s := range resp.Sessions {
		vlan := fmt.Sprintf("%d", s.OuterVlan)
		if s.InnerVlan > 0 {
			vlan = fmt.Sprintf("%d.%d", s.OuterVlan, s.InnerVlan)
		}

		ip := s.Ipv4Address
		if ip == "" {
			ip = s.Ipv6Address
		}
		if ip == "" {
			ip = "-"
		}

		sessionID := s.AcctSessionId
		if sessionID == "" {
			sessionID = s.SessionId
		}

		hostname := s.Hostname
		if hostname == "" {
			hostname = "-"
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			sessionID, s.Mac, vlan, s.State, s.Protocol, ip, hostname)
	}

	w.Flush()
	fmt.Printf("\nTotal: %d sessions\n", len(resp.Sessions))
	return nil
}

func cmdShowSession(ctx context.Context, cli *CLI, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: show session <session-id>")
	}

	req := &bngpb.GetSessionRequest{
		SessionId: args[0],
	}

	session, err := cli.client.GetSession(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	fmt.Println(string(data))
	return nil
}

func cmdShowStats(ctx context.Context, cli *CLI, args []string) error {
	req := &bngpb.GetStatsRequest{}

	stats, err := cli.client.GetStats(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to get stats: %w", err)
	}

	fmt.Println()
	fmt.Println("BNG Statistics")
	fmt.Println(strings.Repeat("=", 50))
	fmt.Printf("Total Sessions:       %d\n", stats.TotalSessions)
	fmt.Printf("  Active:             %d\n", stats.ActiveSessions)
	fmt.Printf("  Released:           %d\n", stats.ReleasedSessions)
	fmt.Println()
	fmt.Printf("IPoE DHCPv4:          %d\n", stats.IpoeV4Sessions)
	fmt.Printf("IPoE DHCPv6:          %d\n", stats.IpoeV6Sessions)
	fmt.Printf("PPP:                  %d\n", stats.PppSessions)
	fmt.Println()

	return nil
}

func cmdShowSubscriberSessions(ctx context.Context, cli *CLI, args []string) error {
	return executeShowHandler(ctx, cli, paths.SubscriberSessions, args)
}

func cmdShowSubscriberSession(ctx context.Context, cli *CLI, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("session-id required")
	}
	return executeShowHandler(ctx, cli, paths.SubscriberSession, args)
}

func cmdShowSubscriberStats(ctx context.Context, cli *CLI, args []string) error {
	return executeShowHandler(ctx, cli, paths.SubscriberStats, args)
}

func cmdShowAAARadiusServers(ctx context.Context, cli *CLI, args []string) error {
	return executeShowHandler(ctx, cli, paths.AAARadiusServers, args)
}

func cmdShowSystemThreads(ctx context.Context, cli *CLI, args []string) error {
	return executeShowHandler(ctx, cli, paths.SystemThreads, args)
}

func cmdShowVRFs(ctx context.Context, cli *CLI, args []string) error {
	return executeShowHandler(ctx, cli, paths.VRFS, args)
}

func cmdShowProtocolsBGPStatistics(ctx context.Context, cli *CLI, args []string) error {
	return executeShowHandler(ctx, cli, paths.ProtocolsBGPStatistics, args)
}

func cmdShowProtocolsBGPIPv6Statistics(ctx context.Context, cli *CLI, args []string) error {
	return executeShowHandler(ctx, cli, paths.ProtocolsBGPIPv6Statistics, args)
}

func executeShowHandler(ctx context.Context, cli *CLI, path paths.Path, args []string) error {
	req := &bngpb.GetOperationalStatsRequest{
		Path: path.String(),
	}

	resp, err := cli.client.GetOperationalStats(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to get stats: %w", err)
	}

	format := FormatCLI
	for i := 0; i < len(args); i++ {
		if args[i] == "|" && i+1 < len(args) {
			switch args[i+1] {
			case "json":
				format = FormatJSON
			case "yaml":
				format = FormatYAML
			case "cli":
				format = FormatCLI
			default:
				return fmt.Errorf("unsupported format: %s", args[i+1])
			}
			break
		}
	}

	rawData := resp.Metrics[path.String()]
	var data interface{}
	if err := json.Unmarshal(rawData, &data); err != nil {
		return fmt.Errorf("unmarshal data: %w", err)
	}

	formatter := NewGenericFormatter()
	output, err := formatter.Format(data, format)
	if err != nil {
		return fmt.Errorf("format output: %w", err)
	}

	fmt.Print(output)
	return nil
}

func cmdShowIPTable(ctx context.Context, cli *CLI, args []string) error {
	return executeShowHandler(ctx, cli, paths.IPTable, args)
}

func cmdSessionTerminate(ctx context.Context, cli *CLI, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: session terminate <session-id>")
	}

	req := &bngpb.TerminateSessionRequest{
		SessionId: args[0],
	}

	resp, err := cli.client.TerminateSession(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to terminate session: %w", err)
	}

	if resp.Success {
		fmt.Printf("Session %s terminated successfully\n", args[0])
	} else {
		return fmt.Errorf("failed to terminate session: %s", resp.Message)
	}

	return nil
}

func cmdClearScreen(ctx context.Context, cli *CLI, args []string) error {
	fmt.Print("\033[H\033[2J")
	return nil
}

func cmdHelp(ctx context.Context, cli *CLI, args []string) error {
	fmt.Println("\nosvbng Interactive CLI")
	fmt.Println(strings.Repeat("=", 50))
	fmt.Println("\nType '?' at any point for context-sensitive help")
	fmt.Println("Use Tab for auto-completion")
	fmt.Println()

	cli.tree.ShowHelp("", cli.devMode)

	fmt.Println("Built-in commands:")
	fmt.Println("  exit, quit                 Exit the CLI")
	fmt.Println()

	return nil
}

func cmdShowRunningConfig(ctx context.Context, cli *CLI, args []string) error {
	req := &bngpb.GetRunningConfigRequest{}

	resp, err := cli.client.GetRunningConfig(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to get running config: %w", err)
	}

	fmt.Println(resp.ConfigYaml)
	return nil
}

func cmdShowStartupConfig(ctx context.Context, cli *CLI, args []string) error {
	req := &bngpb.GetStartupConfigRequest{}

	resp, err := cli.client.GetStartupConfig(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to get startup config: %w", err)
	}

	fmt.Println(resp.ConfigYaml)
	return nil
}

func cmdShowConfigHistory(ctx context.Context, cli *CLI, args []string) error {
	req := &bngpb.ListVersionsRequest{}

	resp, err := cli.client.ListVersions(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to list versions: %w", err)
	}

	if len(resp.Versions) == 0 {
		fmt.Println("No configuration versions found")
		return nil
	}

	fmt.Println()
	fmt.Println("Configuration Version History")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "VERSION\tTIMESTAMP\tCOMMIT MESSAGE\tCHANGES")

	for _, v := range resp.Versions {
		timestamp := time.Unix(v.Timestamp, 0).Format("2006-01-02 15:04:05")
		changeCount := len(v.Changes)
		fmt.Fprintf(w, "%d\t%s\t%s\t%d\n",
			v.Version, timestamp, v.CommitMsg, changeCount)
	}

	w.Flush()
	fmt.Println()
	return nil
}

func cmdShowConfigVersion(ctx context.Context, cli *CLI, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: show config version <version-number>")
	}

	var version int32
	if _, err := fmt.Sscanf(args[0], "%d", &version); err != nil {
		return fmt.Errorf("invalid version number: %s", args[0])
	}

	req := &bngpb.GetVersionRequest{
		Version: version,
	}

	resp, err := cli.client.GetVersion(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to get version: %w", err)
	}

	v := resp.Version
	timestamp := time.Unix(v.Timestamp, 0).Format("2006-01-02 15:04:05")

	fmt.Println()
	fmt.Printf("Configuration Version %d\n", v.Version)
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("Timestamp:    %s\n", timestamp)
	fmt.Printf("Commit Msg:   %s\n", v.CommitMsg)
	fmt.Printf("Changes:      %d\n", len(v.Changes))
	fmt.Println()

	if len(v.Changes) > 0 {
		fmt.Println("Change Details:")
		fmt.Println(strings.Repeat("-", 80))
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "TYPE\tPATH\tVALUE")

		for _, c := range v.Changes {
			value := c.Value
			if len(value) > 50 {
				value = value[:47] + "..."
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", c.Type, c.Path, value)
		}

		w.Flush()
	}

	fmt.Println()
	return nil
}

func cmdConfigure(ctx context.Context, cli *CLI, args []string) error {
	if cli.configMode {
		return fmt.Errorf("already in configuration mode")
	}

	req := &bngpb.ConfigEnterRequest{}
	resp, err := cli.client.ConfigEnter(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to enter config mode: %w", err)
	}

	cli.configMode = true
	cli.configSessionID = resp.SessionId

	fmt.Println("Entered configuration mode")
	fmt.Println("Use 'commit' to apply changes, 'discard' to cancel, 'exit' to leave config mode")
	return nil
}

func cmdSet(ctx context.Context, cli *CLI, args []string) error {
	if !cli.configMode {
		return fmt.Errorf("not in configuration mode. Use 'configure' first")
	}

	if len(args) < 2 {
		return fmt.Errorf("usage: set <path> <value>")
	}

	path := args[0]
	value := strings.Join(args[1:], " ")

	req := &bngpb.ConfigSetRequest{
		SessionId: cli.configSessionID,
		Path:      path,
		Value:     value,
	}

	resp, err := cli.client.ConfigSet(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to set config: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("%s", resp.Message)
	}

	fmt.Printf("Set %s = %s\n", path, value)
	return nil
}

func cmdCommit(ctx context.Context, cli *CLI, args []string) error {
	if !cli.configMode {
		return fmt.Errorf("not in configuration mode")
	}

	commitMsg := "Configuration change"
	if len(args) > 0 {
		commitMsg = strings.Join(args, " ")
	}

	req := &bngpb.ConfigCommitRequest{
		SessionId: cli.configSessionID,
		CommitMsg: commitMsg,
	}

	resp, err := cli.client.ConfigCommit(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("%s", resp.Message)
	}

	fmt.Printf("Configuration committed (version %d)\n", resp.Version)
	cli.configMode = false
	cli.configSessionID = ""
	return nil
}

func cmdDiscard(ctx context.Context, cli *CLI, args []string) error {
	if !cli.configMode {
		return fmt.Errorf("not in configuration mode")
	}

	req := &bngpb.ConfigDiscardRequest{
		SessionId: cli.configSessionID,
	}

	_, err := cli.client.ConfigDiscard(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to discard changes: %w", err)
	}

	fmt.Println("Configuration changes discarded")
	cli.configMode = false
	cli.configSessionID = ""
	return nil
}

func cmdCompare(ctx context.Context, cli *CLI, args []string) error {
	if !cli.configMode {
		return fmt.Errorf("not in configuration mode")
	}

	req := &bngpb.ConfigDiffRequest{
		SessionId: cli.configSessionID,
	}

	resp, err := cli.client.ConfigDiff(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to get diff: %w", err)
	}

	hasChanges := len(resp.Added) > 0 || len(resp.Deleted) > 0 || len(resp.Modified) > 0

	if !hasChanges {
		fmt.Println("No changes")
		return nil
	}

	fmt.Println()
	fmt.Println("Configuration Changes")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println()

	if len(resp.Added) > 0 {
		fmt.Println("Added:")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		for _, line := range resp.Added {
			fmt.Fprintf(w, "  + %s\t= %s\n", line.Path, line.Value)
		}
		w.Flush()
		fmt.Println()
	}

	if len(resp.Modified) > 0 {
		fmt.Println("Modified:")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		for _, line := range resp.Modified {
			fmt.Fprintf(w, "  ~ %s\t= %s\n", line.Path, line.Value)
		}
		w.Flush()
		fmt.Println()
	}

	if len(resp.Deleted) > 0 {
		fmt.Println("Deleted:")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		for _, line := range resp.Deleted {
			fmt.Fprintf(w, "  - %s\t= %s\n", line.Path, line.Value)
		}
		w.Flush()
		fmt.Println()
	}

	return nil
}

func cmdExitConfig(ctx context.Context, cli *CLI, args []string) error {
	if !cli.configMode {
		return fmt.Errorf("not in configuration mode")
	}

	req := &bngpb.ConfigDiscardRequest{
		SessionId: cli.configSessionID,
	}

	_, err := cli.client.ConfigDiscard(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to exit config mode: %w", err)
	}

	fmt.Println("Exited configuration mode (changes discarded)")
	cli.configMode = false
	cli.configSessionID = ""
	return nil
}

func cmdVppctl(ctx context.Context, cli *CLI, args []string) error {
	if !cli.devMode {
		return fmt.Errorf("vppctl command only available in development mode")
	}
	output, err := cli.ExecVPP(args...)
	if err != nil {
		return err
	}
	fmt.Print(output)
	return nil
}
