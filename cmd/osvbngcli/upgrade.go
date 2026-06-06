// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/veesix-networks/osvbng/pkg/upgrade"
)

// upgradeRunner is the subset of *upgrade.Runner the osvbngcli builtin
// actually drives. Declared as an interface so tests can supply a
// mock without standing up a real filesystem sandbox.
type upgradeRunner interface {
	Plan(ctx context.Context, tarballPath string) (*upgrade.PlanResult, error)
	Apply(ctx context.Context, tarballPath string) (*upgrade.ApplyResult, error)
	ApplyOne(ctx context.Context, tarballPath string, opts upgrade.ApplyOptions) (*upgrade.ApplyResult, error)
	Rollback(ctx context.Context) (*upgrade.RollbackResult, error)
	Status(ctx context.Context) (*upgrade.StatusResult, error)
}

// upgradeCtl carries the per-process state the main.go signal handler
// needs to redirect SIGINT from "exit osvbngcli" to "cancel the
// in-flight upgrade". An upgrade-in-flight registers its cancel func;
// the signal handler invokes it on the first signal and falls through
// to the normal os.Exit path only on a second signal.
var upgradeCtl struct {
	mu     sync.Mutex
	cancel context.CancelFunc
}

// setUpgradeCancel registers cancel as the upgrade-cancel function.
// Returns a cleanup func the caller defers; the cleanup clears the
// state so a later SIGINT outside an upgrade window takes the
// shutdown path.
func setUpgradeCancel(cancel context.CancelFunc) func() {
	upgradeCtl.mu.Lock()
	upgradeCtl.cancel = cancel
	upgradeCtl.mu.Unlock()
	return func() {
		upgradeCtl.mu.Lock()
		upgradeCtl.cancel = nil
		upgradeCtl.mu.Unlock()
	}
}

// interruptActiveUpgrade is the hook main.go's signal handler calls.
// Returns true if an upgrade was in flight and got its context
// cancelled; false when no upgrade is running and the caller should
// fall through to the normal osvbngcli shutdown path.
func interruptActiveUpgrade() bool {
	upgradeCtl.mu.Lock()
	cancel := upgradeCtl.cancel
	upgradeCtl.mu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
}

// handleUpgrade is the dispatcher for the `upgrade` osvbngcli builtin. It
// parses the sub-action, constructs a Runner with the osvbngcli-side
// ProgressReporter, and routes to the matching Runner method.
func (c *CLI) handleUpgrade(invocation *Invocation) error {
	if len(invocation.PathTokens) < 2 {
		return upgradeUsageError("missing sub-action")
	}
	subAction := invocation.PathTokens[1]
	args := invocation.PathTokens[2:]

	runner := newReplUpgradeRunner()

	return runUpgradeAction(context.Background(), runner, subAction, args)
}

// runUpgradeAction is the testable inner core. Takes an upgradeRunner
// interface so unit tests can supply a fake without touching the
// filesystem.
func runUpgradeAction(ctx context.Context, runner upgradeRunner, subAction string, args []string) error {
	switch subAction {
	case "plan":
		if len(args) != 1 {
			return upgradeUsageError("plan requires exactly one tarball path argument")
		}
		ctx, cancel := context.WithCancel(ctx)
		cleanup := setUpgradeCancel(cancel)
		defer cleanup()
		defer cancel()

		plan, err := runner.Plan(ctx, args[0])
		if err != nil {
			return err
		}
		renderPlan(plan)
		return nil

	case "apply":
		tarballPath, opts, err := parseApplyArgs(args)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithCancel(ctx)
		cleanup := setUpgradeCancel(cancel)
		defer cleanup()
		defer cancel()

		res, err := runner.ApplyOne(ctx, tarballPath, opts)
		if err != nil {
			return err
		}
		renderApply(res)
		return nil

	case "rollback":
		if len(args) != 0 {
			return upgradeUsageError("rollback takes no arguments")
		}
		ctx, cancel := context.WithCancel(ctx)
		cleanup := setUpgradeCancel(cancel)
		defer cleanup()
		defer cancel()

		res, err := runner.Rollback(ctx)
		if err != nil {
			return err
		}
		renderRollback(res)
		return nil

	case "status":
		if len(args) != 0 {
			return upgradeUsageError("status takes no arguments")
		}
		st, err := runner.Status(ctx)
		if err != nil {
			return err
		}
		renderStatus(st)
		return nil

	default:
		return upgradeUsageError(fmt.Sprintf("unknown sub-action %q", subAction))
	}
}

// newReplUpgradeRunner returns a Runner configured for interactive osvbngcli
// use: production paths + osvbngcli reporter. Tests use a mocked
// upgradeRunner instead of this function.
func newReplUpgradeRunner() *upgrade.Runner {
	return upgrade.NewRunner(newReplReporter())
}

// upgradeUsageError formats a usage error consistently across sub-actions.
func upgradeUsageError(reason string) error {
	return fmt.Errorf("%s\nusage:\n  upgrade plan <tarball>\n  upgrade apply <tarball>\n  upgrade rollback\n  upgrade status", reason)
}

// --- render helpers ---

func renderPlan(p *upgrade.PlanResult) {
	fmt.Println()
	fmt.Printf("Tier:                  %s\n", p.Tier)
	fmt.Printf("From version:          %s\n", versionOrUnknown(p.From))
	fmt.Printf("To version:            %s\n", p.To)
	fmt.Printf("Estimated outage:      %ds\n", p.EstimatedOutageSec)
	fmt.Printf("Rollback available:    %v\n", p.RollbackAvailable)
	fmt.Println()
	fmt.Printf("Artifacts changing (%d):\n", len(p.Artifacts))
	for _, a := range p.Artifacts {
		fmt.Printf("  %s\n", a.Path)
	}
	if len(p.DriftFindings) > 0 {
		fmt.Println()
		fmt.Printf("Drift warnings (%d):\n", len(p.DriftFindings))
		for _, d := range p.DriftFindings {
			fmt.Printf("  %s\n", d.String())
		}
	}
	fmt.Println()
	fmt.Println("Run `upgrade apply <tarball>` to apply.")
}

func renderApply(r *upgrade.ApplyResult) {
	fmt.Println()
	fmt.Printf("Upgrade complete: %s → %s\n", versionOrUnknown(r.From), r.To)
	fmt.Printf("Snapshot: %s\n", r.SnapshotDir)
	fmt.Printf("Artifacts swapped: %d\n", len(r.ArtifactsSwap))
	fmt.Printf("Health: %s\n", r.HealthOutcome)
}

func renderRollback(r *upgrade.RollbackResult) {
	fmt.Println()
	fmt.Printf("Rollback complete: %s → %s\n", r.From, r.To)
	fmt.Printf("Restored files: %d\n", len(r.RestoredFiles))
	fmt.Printf("Health: %s\n", r.HealthOutcome)
}

func renderStatus(s *upgrade.StatusResult) {
	fmt.Println()
	fmt.Printf("Current version:       %s\n", versionOrUnknown(s.CurrentVersion))
	if s.LastUpgrade != nil {
		fmt.Printf("Last upgrade:          %s (from %s, phase=%s)\n",
			s.LastUpgrade.UpdatedAt.Format("2006-01-02T15:04:05Z"),
			s.LastUpgrade.From, s.LastUpgrade.Phase)
	} else {
		fmt.Println("Last upgrade:          (none)")
	}
	if s.RollbackAvailable {
		sort.Strings(s.RollbackVersions)
		fmt.Printf("Rollback available:    yes (%s)\n", strings.Join(s.RollbackVersions, ", "))
	} else {
		fmt.Println("Rollback available:    no")
	}
}

func versionOrUnknown(s string) string {
	if s == "" {
		return "(unknown)"
	}
	return s
}

func parseApplyArgs(args []string) (string, upgrade.ApplyOptions, error) {
	var opts upgrade.ApplyOptions
	var positional []string
	for _, a := range args {
		switch a {
		case "--first-boot":
			opts.FirstBoot = true
		case "--force-retry":
			opts.ForceRetry = true
		default:
			if strings.HasPrefix(a, "--") {
				return "", opts, upgradeUsageError(fmt.Sprintf("unknown flag %q", a))
			}
			positional = append(positional, a)
		}
	}
	if len(positional) != 1 {
		return "", opts, upgradeUsageError("apply requires exactly one tarball path argument")
	}
	return positional[0], opts, nil
}
