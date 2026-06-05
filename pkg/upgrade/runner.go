// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package upgrade

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Runner stitches the upgrade primitives (manifest parse, stage,
// signature verify, snapshot, swap, supervise, health) into the
// operator-facing Plan / Apply / Rollback / Status methods.
type Runner struct {
	// System paths. Defaults are filled in by NewRunner; tests
	// override.
	BinaryPath    string // /usr/local/bin/osvbngd
	CLIPath       string // /usr/local/bin/osvbngcli
	PluginDir     string // /usr/lib/x86_64-linux-gnu/vpp_plugins/
	TemplateDir   string // /usr/share/osvbng/templates/
	StateRoot     string // /var/opt/osvbng/
	RollbackRoot  string // /var/opt/osvbng/rollback/
	QuarantineDir string // /var/opt/osvbng/quarantine/
	StateFile     string // /run/osvbng/state
	SystemdUnit   string // osvbng.service
	DropInRoot    string // /run/systemd/system — parent of <unit>.d for the transient Restart= override
	PubKey        string // /etc/osvbng/release-keys/cosign.pub

	// Behaviour knobs. Zero values pick spec defaults.
	HealthTimeout  time.Duration
	StallLimit     time.Duration
	PollInterval   time.Duration
	StateFileGrace time.Duration

	// Injectables.
	Cmd      Commander
	Reporter ProgressReporter
}

// NewRunner returns a Runner with production defaults wired in.
func NewRunner(reporter ProgressReporter) *Runner {
	r := &Runner{
		BinaryPath:    "/usr/local/bin/osvbngd",
		CLIPath:       "/usr/local/bin/osvbngcli",
		PluginDir:     "/usr/lib/x86_64-linux-gnu/vpp_plugins",
		TemplateDir:   "/usr/share/osvbng/templates",
		StateRoot:     "/var/opt/osvbng",
		RollbackRoot:  "/var/opt/osvbng/rollback",
		QuarantineDir: "/var/opt/osvbng/quarantine",
		StateFile:     "/run/osvbng/state",
		SystemdUnit:   "osvbng.service",
		DropInRoot:    "/run/systemd/system",
		PubKey:        "/etc/osvbng/release-keys/cosign.pub",
		Cmd:           execCommander{},
		Reporter:      reporter,
	}
	if r.Reporter == nil {
		r.Reporter = NullReporter{}
	}
	return r
}

// PlanResult describes what an Apply call would do without performing
// any side effects on production paths.
type PlanResult struct {
	From               string
	To                 string
	Tier               string
	Artifacts          []ManifestArtifact
	EstimatedOutageSec int
	DriftFindings      []DriftFinding
	RollbackAvailable  bool
}

// StatusResult is what Status returns: a snapshot of installed
// version, last upgrade outcome, in-flight journal, and rollback
// availability.
type StatusResult struct {
	CurrentVersion    string
	LastUpgrade       *JournalState // nil when no journal exists
	RollbackAvailable bool
	RollbackVersions  []string
}

// RollbackResult is what an explicit or auto Rollback returns.
type RollbackResult struct {
	From            string
	To              string
	HealthOutcome   string
	RestoredFiles   []string
	JournalEndPhase string
}

// Plan is the read-only dry-run. Extracts to staging, verifies the
// signature, parses the manifest, checks drift — and removes staging
// at the end. No side effects on production paths.
func (r *Runner) Plan(ctx context.Context, tarballPath string) (*PlanResult, error) {
	r.Reporter.Stage(1, 5, "Verifying signature")
	if err := r.verifySignature(tarballPath); err != nil {
		return nil, err
	}
	r.Reporter.Progress("OK")

	r.Reporter.Stage(2, 5, "Extracting tarball (preview)")
	staging, err := ExtractTarball(tarballPath)
	if err != nil {
		return nil, err
	}
	defer staging.Cleanup()
	r.Reporter.Progress(fmt.Sprintf("OK (%d files)", len(staging.Hashes)))

	r.Reporter.Stage(3, 5, "Parsing manifest")
	if mismatches, err := staging.CrossCheckArtifacts(); err != nil {
		return nil, err
	} else if len(mismatches) > 0 {
		return nil, fmt.Errorf("artifact sha256 cross-check failed for %d file(s): %v", len(mismatches), mismatches)
	}

	from, _ := r.discoverCurrentVersion(ctx)

	r.Reporter.Stage(4, 5, "Checking drift")
	drift, err := DetectDrift(staging.Manifest)
	if err != nil {
		return nil, err
	}
	for _, d := range drift {
		r.Reporter.Warn(d.String())
	}

	r.Reporter.Stage(5, 5, "Computing impact")
	rbAvail := r.hasAnySnapshot()

	return &PlanResult{
		From:               from,
		To:                 staging.Manifest.OsvbngVersion,
		Tier:               staging.Manifest.Type,
		Artifacts:          staging.Manifest.Artifacts,
		EstimatedOutageSec: staging.Manifest.EstimatedOutageSec,
		DriftFindings:      drift,
		RollbackAvailable:  rbAvail,
	}, nil
}

// Apply applies a single tarball and orchestrates the full apply flow.
// Wraps ApplyOne with zero ApplyOptions for the single-tarball UX.
func (r *Runner) Apply(ctx context.Context, tarballPath string) (*ApplyResult, error) {
	return r.ApplyOne(ctx, tarballPath, ApplyOptions{})
}

// ApplyOne is the apply primitive a future chain coordinator
// (issue #114) will call repeatedly. The single-tarball Apply is a
// thin wrapper.
func (r *Runner) ApplyOne(ctx context.Context, tarballPath string, opts ApplyOptions) (*ApplyResult, error) {
	r.defaults()

	if err := r.ensureStateDirs(); err != nil {
		return nil, err
	}

	const totalStages = 14

	r.Reporter.Stage(1, totalStages, "Staging tarball")
	staging, err := ExtractTarball(tarballPath)
	if err != nil {
		return nil, err
	}
	defer staging.Cleanup()
	r.Reporter.Progress(fmt.Sprintf("extracted %d files into %s", len(staging.Hashes), staging.Dir))

	r.Reporter.Stage(2, totalStages, "Verifying signature")
	if err := r.verifySignature(tarballPath); err != nil {
		if _, qErr := Quarantine(r.QuarantineDir, tarballPath, fmt.Sprintf("signature verification failed: %v", err)); qErr != nil {
			r.Reporter.Warn(fmt.Sprintf("quarantine attempt failed: %v", qErr))
		}
		return nil, err
	}
	r.Reporter.Progress("OK")

	r.Reporter.Stage(3, totalStages, "Parsing manifest")
	manifest := staging.Manifest
	if mismatches, err := staging.CrossCheckArtifacts(); err != nil {
		return nil, err
	} else if len(mismatches) > 0 {
		_, _ = Quarantine(r.QuarantineDir, tarballPath, fmt.Sprintf("verified signature but artifact digest mismatch: %v", mismatches))
		return nil, fmt.Errorf("verified signature but artifact digest mismatch: %v", mismatches)
	}

	from, _ := r.discoverCurrentVersion(ctx)
	if opts.ExpectedFrom != "" && from != opts.ExpectedFrom {
		return nil, fmt.Errorf("apply: chain coordinator expected from=%q but on-disk version is %q", opts.ExpectedFrom, from)
	}
	r.Reporter.Progress(fmt.Sprintf("from %s → %s (Tier %s)", from, manifest.OsvbngVersion, manifest.Type))

	r.Reporter.Stage(4, totalStages, "Checking drift")
	if drift, err := DetectDrift(manifest); err != nil {
		return nil, err
	} else {
		for _, d := range drift {
			r.Reporter.Warn(d.String())
		}
	}

	// Begin recording the journal as we cross irreversible boundaries.
	journal := NewJournal(filepath.Join(r.StateRoot, "upgrade-state.json"))
	if err := journal.Write(&JournalState{
		From:      from,
		To:        manifest.OsvbngVersion,
		Tarball:   tarballPath,
		StartedAt: time.Now().UTC(),
		Phase:     "started",
	}); err != nil {
		return nil, fmt.Errorf("write initial journal: %w", err)
	}

	r.Reporter.Stage(5, totalStages, "Snapshotting current version")
	snapDir, _, err := Snapshot(r.RollbackRoot, from, manifest.OsvbngVersion, manifest)
	if err != nil {
		return nil, err
	}
	if err := journal.SetPhase("snapshot_done"); err != nil {
		return nil, err
	}
	r.Reporter.Progress(snapDir)

	r.Reporter.Stage(6, totalStages, "Pre-apply hook")
	if err := r.runHook(ctx, staging.Dir, manifest.Hooks.Pre.Path, from, manifest.OsvbngVersion); err != nil {
		return nil, fmt.Errorf("pre-apply hook: %w", err)
	}
	if err := journal.SetPhase("pre_hook_done"); err != nil {
		return nil, err
	}

	supervisor := r.supervisor()
	r.Reporter.Stage(7, totalStages, "Suspending systemd auto-restart")
	if err := supervisor.SuspendAutoRestart(ctx); err != nil {
		return nil, err
	}
	defer func() {
		if err := supervisor.RestoreAutoRestart(ctx); err != nil {
			r.Reporter.Warn(fmt.Sprintf("RestoreAutoRestart on exit: %v", err))
		}
	}()
	if err := journal.SetPhase("restart_suspended"); err != nil {
		return nil, err
	}

	r.Reporter.Stage(8, totalStages, "Stopping daemon")
	if err := supervisor.Stop(ctx); err != nil {
		return nil, err
	}
	if err := journal.SetPhase("daemon_stopped"); err != nil {
		return nil, err
	}

	r.Reporter.Stage(9, totalStages, "Swapping artifacts")
	swapped, err := r.swapArtifacts(ctx, staging, manifest, journal)
	if err != nil {
		// Best-effort rollback of whatever swapped before the failure;
		// attempt to restart the daemon either way so the operator is
		// not left with a stopped service.
		r.Reporter.Warn(fmt.Sprintf("swap loop failed after %d artifact(s): %v", len(swapped), err))
		_ = journal.SetPhase("aborted_mid_swap")
		return r.rollbackAfterFailedApply(ctx, supervisor, journal, "swap loop failure")
	}

	// VPP keeps plugins loaded in memory; a fresh .so on disk only
	// takes effect after a VPP restart. osvbng.service is already
	// stopped so there's no cascade to worry about.
	if r.swapTouchesPlugins(swapped) {
		r.Reporter.Progress("plugin changed, restarting VPP")
		if err := r.restartVPP(ctx); err != nil {
			_ = journal.SetPhase("aborted_post_swap")
			return r.rollbackAfterFailedApply(ctx, supervisor, journal, fmt.Sprintf("vpp restart failed: %v", err))
		}
	}

	r.Reporter.Stage(10, totalStages, "Starting daemon")
	if err := supervisor.Start(ctx); err != nil {
		_ = journal.SetPhase("aborted_post_swap")
		return r.rollbackAfterFailedApply(ctx, supervisor, journal, "systemctl start failed")
	}
	if err := journal.SetPhase("daemon_started"); err != nil {
		return nil, err
	}

	r.Reporter.Stage(11, totalStages, "Waiting for daemon to become ready")
	healthOutcome, healthMsg := r.waitHealthy(ctx, supervisor, manifest.OsvbngVersion)
	if healthOutcome != HealthOK {
		r.Reporter.Warn(fmt.Sprintf("health failed: %s — %s", healthOutcome, healthMsg))
		_ = journal.SetPhase("health_failed")
		return r.rollbackAfterFailedApply(ctx, supervisor, journal, fmt.Sprintf("health %s: %s", healthOutcome, healthMsg))
	}

	r.Reporter.Stage(12, totalStages, "Committing new version")
	if err := WriteCurrentManifest(r.StateRoot, manifest); err != nil {
		return nil, fmt.Errorf("write current-manifest: %w", err)
	}
	if err := journal.SetPhase("completed"); err != nil {
		return nil, err
	}

	r.Reporter.Stage(13, totalStages, "Pruning old snapshots")
	if opts.PrunePolicy == PruneKeepN {
		keep := opts.KeepN
		if keep == 0 {
			keep = 1
		}
		if err := PruneSnapshots(r.RollbackRoot, keep); err != nil {
			r.Reporter.Warn(fmt.Sprintf("prune snapshots: %v", err))
		}
	}

	r.Reporter.Stage(14, totalStages, "Post-apply hook")
	if err := r.runHook(ctx, staging.Dir, manifest.Hooks.Post.Path, from, manifest.OsvbngVersion); err != nil {
		r.Reporter.Warn(fmt.Sprintf("post-apply hook returned non-zero (apply itself succeeded): %v", err))
	}

	return &ApplyResult{
		From:            from,
		To:              manifest.OsvbngVersion,
		SnapshotDir:     snapDir,
		ArtifactsSwap:   swapped,
		HealthOutcome:   healthOutcome.String(),
		JournalEndPhase: "completed",
	}, nil
}

// Rollback restores the most recent snapshot. Reads the journal to
// figure out what to restore, suspends systemd Restart=, stops the
// daemon, replays the snapshot's metadata + bytes, restarts, and
// health-polls. Idempotent: a missing journal yields "no rollback
// available" not an error.
func (r *Runner) Rollback(ctx context.Context) (*RollbackResult, error) {
	r.defaults()

	if err := r.ensureStateDirs(); err != nil {
		return nil, err
	}

	journal := NewJournal(filepath.Join(r.StateRoot, "upgrade-state.json"))
	state, err := journal.Read()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, errors.New("rollback: no upgrade journal present; nothing to rollback")
		}
		return nil, fmt.Errorf("read journal: %w", err)
	}

	from := state.From
	to := state.To
	snapDir := filepath.Join(r.RollbackRoot, from)
	meta, err := LoadSnapshotMetadata(snapDir)
	if err != nil {
		return nil, fmt.Errorf("rollback: load snapshot metadata: %w", err)
	}

	supervisor := r.supervisor()

	const totalStages = 5
	r.Reporter.Stage(1, totalStages, "Suspending systemd auto-restart")
	if err := supervisor.SuspendAutoRestart(ctx); err != nil {
		return nil, err
	}
	defer func() {
		if err := supervisor.RestoreAutoRestart(ctx); err != nil {
			r.Reporter.Warn(fmt.Sprintf("RestoreAutoRestart on exit: %v", err))
		}
	}()

	r.Reporter.Stage(2, totalStages, "Stopping daemon")
	if err := supervisor.Stop(ctx); err != nil {
		return nil, err
	}

	r.Reporter.Stage(3, totalStages, "Restoring artifacts from snapshot")
	restored, err := r.restoreFromSnapshot(snapDir, meta)
	if err != nil {
		_ = journal.SetPhase("rollback_failed")
		return nil, err
	}

	if r.swapTouchesPlugins(restored) {
		r.Reporter.Progress("plugin restored, restarting VPP")
		if err := r.restartVPP(ctx); err != nil {
			_ = journal.SetPhase("rollback_failed")
			return nil, fmt.Errorf("vpp restart during rollback: %w", err)
		}
	}

	r.Reporter.Stage(4, totalStages, "Starting daemon")
	if err := supervisor.Start(ctx); err != nil {
		_ = journal.SetPhase("rollback_failed")
		return nil, err
	}

	r.Reporter.Stage(5, totalStages, "Waiting for daemon to become ready")
	outcome, msg := r.waitHealthy(ctx, supervisor, from)
	if outcome != HealthOK {
		_ = journal.SetPhase("rollback_failed")
		return nil, fmt.Errorf("rollback health failed: %s — %s", outcome, msg)
	}

	_ = journal.SetPhase("rolled_back")

	return &RollbackResult{
		From:            to,
		To:              from,
		HealthOutcome:   outcome.String(),
		RestoredFiles:   restored,
		JournalEndPhase: "rolled_back",
	}, nil
}

// Status reads the journal and rollback dir to surface the current
// installed version + last upgrade outcome + available rollbacks.
// Tolerates missing state directories (never-upgraded box).
func (r *Runner) Status(ctx context.Context) (*StatusResult, error) {
	r.defaults()

	current, _ := r.discoverCurrentVersion(ctx)

	res := &StatusResult{CurrentVersion: current}

	journalPath := filepath.Join(r.StateRoot, "upgrade-state.json")
	if state, err := NewJournal(journalPath).Read(); err == nil {
		res.LastUpgrade = state
	}

	if entries, err := os.ReadDir(r.RollbackRoot); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				res.RollbackVersions = append(res.RollbackVersions, e.Name())
			}
		}
		res.RollbackAvailable = len(res.RollbackVersions) > 0
	}

	return res, nil
}

// --- internal helpers ---

func (r *Runner) defaults() {
	if r.HealthTimeout == 0 {
		r.HealthTimeout = 60 * time.Second
	}
	if r.StallLimit == 0 {
		r.StallLimit = 30 * time.Second
	}
	if r.PollInterval == 0 {
		r.PollInterval = 1 * time.Second
	}
	if r.StateFileGrace == 0 {
		r.StateFileGrace = 5 * time.Second
	}
	if r.Cmd == nil {
		r.Cmd = execCommander{}
	}
	if r.Reporter == nil {
		r.Reporter = NullReporter{}
	}
}

func (r *Runner) supervisor() *Supervisor {
	root := r.DropInRoot
	if root == "" {
		root = "/run/systemd/system"
	}
	return &Supervisor{
		Unit:       r.SystemdUnit,
		DropInRoot: root,
		Cmd:        r.Cmd,
	}
}

func (r *Runner) ensureStateDirs() error {
	for _, dir := range []string{r.StateRoot, r.RollbackRoot, r.QuarantineDir} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}
	return nil
}

func (r *Runner) hasAnySnapshot() bool {
	entries, err := os.ReadDir(r.RollbackRoot)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			return true
		}
	}
	return false
}

func (r *Runner) verifySignature(tarballPath string) error {
	scheme, err := VerifyTarballSignature(tarballPath, r.PubKey)
	if err != nil {
		return err
	}
	if scheme != "" {
		r.Reporter.Progress(fmt.Sprintf("scheme %s", scheme))
	}
	return nil
}

func (r *Runner) discoverCurrentVersion(ctx context.Context) (string, error) {
	return CurrentInstalledVersion(ctx, r.StateRoot, r.BinaryPath, r.Cmd)
}

func (r *Runner) swapTouchesPlugins(paths []string) bool {
	if r.PluginDir == "" {
		return false
	}
	prefix := strings.TrimRight(r.PluginDir, "/") + "/"
	for _, p := range paths {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

func (r *Runner) restartVPP(ctx context.Context) error {
	out, err := r.Cmd.Run(ctx, "systemctl", "restart", "vpp.service")
	if err != nil {
		return fmt.Errorf("systemctl restart vpp.service: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (r *Runner) waitHealthy(ctx context.Context, sv *Supervisor, expectedVersion string) (HealthResult, string) {
	hc := &HealthChecker{
		Supervisor:      sv,
		StateFilePath:   r.StateFile,
		ExpectedVersion: expectedVersion,
		OverallTimeout:  r.HealthTimeout,
		StallLimit:      r.StallLimit,
		PollInterval:    r.PollInterval,
		StateFileGrace:  r.StateFileGrace,
	}
	out, msg, _ := hc.WaitHealthy(ctx)
	return out, msg
}

func (r *Runner) swapArtifacts(ctx context.Context, staging *Staging, manifest *Manifest, journal *Journal) ([]string, error) {
	var swapped []string
	pending := make([]string, 0, len(manifest.Artifacts))
	for _, a := range manifest.Artifacts {
		pending = append(pending, a.Path)
	}

	for i, art := range manifest.Artifacts {
		state, err := journal.Read()
		if err != nil {
			return swapped, err
		}
		state.Phase = "swapping:" + art.Path
		state.CompletedArtifacts = append([]string{}, swapped...)
		state.PendingArtifacts = append([]string{}, pending[i:]...)
		if err := journal.Write(state); err != nil {
			return swapped, err
		}

		srcPath := filepath.Join(staging.Dir, art.Source)
		if err := SwapArtifact(srcPath, art.Path, art.UID, art.GID, art.Mode); err != nil {
			return swapped, fmt.Errorf("swap %s: %w", art.Path, err)
		}

		swapped = append(swapped, art.Path)
		state, err = journal.Read()
		if err != nil {
			return swapped, err
		}
		state.Phase = "swapped:" + art.Path
		state.CompletedArtifacts = append([]string{}, swapped...)
		state.PendingArtifacts = append([]string{}, pending[i+1:]...)
		if err := journal.Write(state); err != nil {
			return swapped, err
		}
		_ = ctx
	}
	return swapped, nil
}

func (r *Runner) rollbackAfterFailedApply(ctx context.Context, sv *Supervisor, journal *Journal, reason string) (*ApplyResult, error) {
	r.Reporter.Warn("triggering auto-rollback: " + reason)
	rb, err := r.Rollback(ctx)
	if err != nil {
		_ = journal.SetPhase("rollback_failed")
		return nil, fmt.Errorf("apply failed and auto-rollback also failed: %w", err)
	}
	return nil, fmt.Errorf("apply failed (%s); auto-rollback succeeded to %s", reason, rb.To)
}

func (r *Runner) restoreFromSnapshot(snapDir string, meta *SnapshotMetadata) ([]string, error) {
	var restored []string
	for i := len(meta.Entries) - 1; i >= 0; i-- {
		entry := meta.Entries[i]
		if !entry.Present {
			// The artifact didn't exist before the upgrade — remove
			// whatever the new version put there.
			if err := os.Remove(entry.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return restored, fmt.Errorf("remove %s during rollback: %w", entry.Path, err)
			}
			restored = append(restored, entry.Path)
			continue
		}

		switch entry.Kind {
		case ArtifactKindSymlink:
			_ = os.Remove(entry.Path)
			if err := os.Symlink(entry.SymlinkTarget, entry.Path); err != nil {
				return restored, fmt.Errorf("recreate symlink %s -> %s: %w", entry.Path, entry.SymlinkTarget, err)
			}
			_ = os.Lchown(entry.Path, entry.UID, entry.GID)
		case ArtifactKindRegular:
			backup := filepath.Join(snapDir, entry.BackupRelpath)
			modeStr := fmt.Sprintf("%04o", entry.Mode)
			if err := SwapArtifact(backup, entry.Path, entry.UID, entry.GID, modeStr); err != nil {
				return restored, fmt.Errorf("restore %s: %w", entry.Path, err)
			}
		default:
			return restored, fmt.Errorf("snapshot entry %s has unknown kind %q", entry.Path, entry.Kind)
		}
		restored = append(restored, entry.Path)
	}
	return restored, nil
}

func (r *Runner) runHook(ctx context.Context, stagingDir, hookRel string, fromVersion, toVersion string) error {
	if hookRel == "" {
		return nil
	}
	hookPath := filepath.Join(stagingDir, hookRel)
	if _, err := os.Stat(hookPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("hook %s declared but not present in tarball", hookRel)
		}
		return err
	}

	cmd := exec.CommandContext(ctx, hookPath)
	cmd.Dir = stagingDir
	cmd.Env = []string{
		"PATH=/usr/sbin:/usr/bin:/sbin:/bin",
		"OSVBNG_UPGRADE_FROM=" + fromVersion,
		"OSVBNG_UPGRADE_TO=" + toVersion,
		"OSVBNG_UPGRADE_STAGING_DIR=" + stagingDir,
	}
	out, err := cmd.CombinedOutput()
	if len(out) > 0 {
		r.Reporter.Detail(string(out))
	}
	if err != nil {
		return fmt.Errorf("hook %s exited non-zero: %w", hookRel, err)
	}
	return nil
}
