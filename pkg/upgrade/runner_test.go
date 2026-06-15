// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package upgrade

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testHarness sets up a complete runtime sandbox for Runner tests:
// real filesystem under a tempdir mimicking production paths, a
// signing keypair, a fake Commander, a recording reporter.
type testHarness struct {
	t        *testing.T
	dir      string
	runner   *Runner
	cmd      *fakeCommander
	reporter *RecordingReporter

	// Paths the tests reference.
	binaryPath  string
	cliPath     string
	pluginDir   string
	templateDir string
	stateRoot   string

	// Signing
	privKey   *ecdsa.PrivateKey
	pubKeyPEM []byte
}

func newHarness(t *testing.T) *testHarness {
	t.Helper()
	dir := t.TempDir()
	binDir := filepath.Join(dir, "usr/local/bin")
	pluginDir := filepath.Join(dir, "usr/lib/x86_64-linux-gnu/vpp_plugins")
	templateDir := filepath.Join(dir, "usr/share/osvbng/templates")
	stateRoot := filepath.Join(dir, "var/opt/osvbng")
	keyDir := filepath.Join(dir, "etc/osvbng/release-keys")
	runDir := filepath.Join(dir, "run/osvbng")
	systemdDir := filepath.Join(dir, "run/systemd/system")

	for _, d := range []string{binDir, pluginDir, templateDir, stateRoot, keyDir, runDir, systemdDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}

	// Plant placeholder current installed binaries so swap has a
	// target to replace. Content must NOT match the manifest's source
	// hash or drift detection would skip them.
	if err := os.WriteFile(filepath.Join(binDir, "osvbngd"), []byte("OLD-osvbngd"), 0o755); err != nil {
		t.Fatalf("plant osvbngd: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "osvbngcli"), []byte("OLD-osvbngcli"), 0o755); err != nil {
		t.Fatalf("plant osvbngcli: %v", err)
	}

	// Pre-create the VPP api socket file so restartVPP's
	// waitVPPSocket finds it immediately in tests. Production uses a
	// real unix socket; tests just need the os.Stat path to succeed.
	if err := os.WriteFile(filepath.Join(runDir, "dataplane_api.sock"), nil, 0o644); err != nil {
		t.Fatalf("plant vpp socket placeholder: %v", err)
	}

	// Generate signing keypair.
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pubDER, _ := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	if err := os.WriteFile(filepath.Join(keyDir, "cosign.pub"), pubPEM, 0o644); err != nil {
		t.Fatalf("write pub key: %v", err)
	}

	cmd := &fakeCommander{}
	reporter := &RecordingReporter{}
	runner := &Runner{
		BinaryPath:    filepath.Join(binDir, "osvbngd"),
		CLIPath:       filepath.Join(binDir, "osvbngcli"),
		PluginDir:     pluginDir,
		TemplateDir:   templateDir,
		StateRoot:     stateRoot,
		RollbackRoot:  filepath.Join(stateRoot, "rollback"),
		QuarantineDir: filepath.Join(stateRoot, "quarantine"),
		StateFile:     filepath.Join(runDir, "state"),
		VPPSocketPath: filepath.Join(runDir, "dataplane_api.sock"),
		SystemdUnit:   "osvbng.service",
		DropInRoot:    systemdDir,
		PubKey:        filepath.Join(keyDir, "cosign.pub"),
		HealthTimeout:   5 * time.Second,
		PollInterval:    10 * time.Millisecond,
		StallLimit:      5 * time.Second,
		VPPStopWait:     50 * time.Millisecond,
		VPPActiveWait:   50 * time.Millisecond,
		VPPPollInterval: 1 * time.Millisecond,
		Cmd:             cmd,
		Reporter:        reporter,
	}

	return &testHarness{
		t:           t,
		dir:         dir,
		runner:      runner,
		cmd:         cmd,
		reporter:    reporter,
		binaryPath:  runner.BinaryPath,
		cliPath:     runner.CLIPath,
		pluginDir:   pluginDir,
		templateDir: templateDir,
		stateRoot:   stateRoot,
		privKey:     priv,
		pubKeyPEM:   pubPEM,
	}
}

// plantFromVersion stores a minimal current-manifest.yaml so
// CurrentInstalledVersion returns the supplied version. Apply tests
// call this before exercising the runner so the from-version
// discovery doesn't depend on a real osvbngd binary being executable.
func (h *testHarness) plantFromVersion(version string) {
	h.t.Helper()
	manifestYAML := fmt.Sprintf(`schema_version: 2
osvbng_version: %s
min_compatible_version: 0.0.0
type: A
build_commit: planted
artifacts:
  - path: %s
    source: osvbngd
    sha256: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
    mode: "0755"
    requires_restart: osvbngd
`, version, h.binaryPath)
	if err := os.MkdirAll(h.stateRoot, 0o755); err != nil {
		h.t.Fatalf("mkdir state root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(h.stateRoot, "current-manifest.yaml"), []byte(manifestYAML), 0o644); err != nil {
		h.t.Fatalf("plant current-manifest: %v", err)
	}
}

func (h *testHarness) writeStateFile(state string, sequence uint64, version string) {
	h.t.Helper()
	payload, _ := json.Marshal(struct {
		State     string    `json:"state"`
		Sequence  uint64    `json:"sequence"`
		UpdatedAt time.Time `json:"updated_at"`
		Version   string    `json:"version,omitempty"`
	}{State: state, Sequence: sequence, UpdatedAt: time.Now().UTC(), Version: version})
	if err := os.WriteFile(h.runner.StateFile, payload, 0o644); err != nil {
		h.t.Fatalf("write state file: %v", err)
	}
}

// buildSignedTarball writes a tarball at tmpdir/<name>.tar.gz with a
// matching .sig sidecar. The manifest declares two artifacts mirroring
// real osvbng-on-disk paths so the swap loop has work to do.
func (h *testHarness) buildSignedTarball(t *testing.T, fromVersion, toVersion string) string {
	t.Helper()
	tarballDir := t.TempDir()
	tarballPath := filepath.Join(tarballDir, fmt.Sprintf("osvbng-v%s.tar.gz", toVersion))

	newOsvbngd := []byte("NEW-osvbngd-binary-" + toVersion)
	newOsvbngcli := []byte("NEW-osvbngcli-binary-" + toVersion)

	// uid/gid set to -1 so tests run as a non-root user; production
	// manifests use 0/0 (root) but the swap layer skips the chown
	// when uid == -1.
	manifestYAML := fmt.Sprintf(`schema_version: 2
osvbng_version: %s
min_compatible_version: %s
type: A
build_commit: testabc
artifacts:
  - path: %s
    source: osvbngd
    sha256: %s
    mode: "0755"
    uid: -1
    gid: -1
    requires_restart: osvbngd
  - path: %s
    source: osvbngcli
    sha256: %s
    mode: "0755"
    uid: -1
    gid: -1
    requires_restart: none
`,
		toVersion,
		fromVersion,
		h.binaryPath, sha256Hex(newOsvbngd),
		h.cliPath, sha256Hex(newOsvbngcli))

	writeTarball(t, tarballDir, filepath.Base(tarballPath), []tarEntry{
		{name: "manifest.yaml", body: []byte(manifestYAML)},
		{name: "osvbngd", body: newOsvbngd, mode: 0o755},
		{name: "osvbngcli", body: newOsvbngcli, mode: 0o755},
	})

	tarballBytes, _ := os.ReadFile(tarballPath)
	digest := sha256.Sum256(tarballBytes)
	sigDER, err := ecdsa.SignASN1(rand.Reader, h.privKey, digest[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := os.WriteFile(tarballPath+".sig",
		[]byte(base64.StdEncoding.EncodeToString(sigDER)+"\n"), 0o644); err != nil {
		t.Fatalf("write sig: %v", err)
	}
	return tarballPath
}

// buildSignedTarballWithPlugin builds a signed tarball that includes
// a VPP plugin alongside the daemon binaries, used to assert the
// VPP-restart code path in Apply.
func (h *testHarness) buildSignedTarballWithPlugin(t *testing.T, fromVersion, toVersion, pluginPath string) string {
	t.Helper()
	tarballDir := t.TempDir()
	tarballPath := filepath.Join(tarballDir, fmt.Sprintf("osvbng-v%s.tar.gz", toVersion))

	newOsvbngd := []byte("NEW-osvbngd-" + toVersion)
	newOsvbngcli := []byte("NEW-osvbngcli-" + toVersion)
	newPlugin := []byte("NEW-plugin-" + toVersion)

	manifestYAML := fmt.Sprintf(`schema_version: 2
osvbng_version: %s
min_compatible_version: %s
type: A
build_commit: testabc
artifacts:
  - path: %s
    source: osvbngd
    sha256: %s
    mode: "0755"
    uid: -1
    gid: -1
    requires_restart: osvbngd
  - path: %s
    source: osvbngcli
    sha256: %s
    mode: "0755"
    uid: -1
    gid: -1
    requires_restart: none
  - path: %s
    source: plugins/osvbng_ipoe_plugin.so
    sha256: %s
    mode: "0644"
    uid: -1
    gid: -1
    requires_restart: vpp
`,
		toVersion, fromVersion,
		h.binaryPath, sha256Hex(newOsvbngd),
		h.cliPath, sha256Hex(newOsvbngcli),
		pluginPath, sha256Hex(newPlugin))

	writeTarball(t, tarballDir, filepath.Base(tarballPath), []tarEntry{
		{name: "manifest.yaml", body: []byte(manifestYAML)},
		{name: "osvbngd", body: newOsvbngd, mode: 0o755},
		{name: "osvbngcli", body: newOsvbngcli, mode: 0o755},
		{name: "plugins/osvbng_ipoe_plugin.so", body: newPlugin, mode: 0o644},
	})

	tarballBytes, _ := os.ReadFile(tarballPath)
	digest := sha256.Sum256(tarballBytes)
	sigDER, err := ecdsa.SignASN1(rand.Reader, h.privKey, digest[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := os.WriteFile(tarballPath+".sig",
		[]byte(base64.StdEncoding.EncodeToString(sigDER)+"\n"), 0o644); err != nil {
		t.Fatalf("write sig: %v", err)
	}
	return tarballPath
}

// configureSystemctlReady arms the fake Commander so every
// "systemctl show" returns "active/running/success". Tests that want
// to model a stop/start/show sequence override this directly.
func (h *testHarness) configureSystemctlReady() {
	h.cmd.scripts = []fakeResp{
		{matchName: "systemctl", out: "ActiveState=active\nSubState=running\nResult=success\n"},
	}
}

// --- tests ---

func TestRunnerPlanHappyPath(t *testing.T) {
	h := newHarness(t)
	tarball := h.buildSignedTarball(t, "0.13.0", "0.13.1")
	h.configureSystemctlReady()

	plan, err := h.runner.Plan(context.Background(), tarball)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if plan.To != "0.13.1" {
		t.Fatalf("plan.To = %q, want 0.13.1", plan.To)
	}
	if plan.Tier != "A" {
		t.Fatalf("plan.Tier = %q, want A", plan.Tier)
	}
	if len(plan.Artifacts) != 2 {
		t.Fatalf("plan.Artifacts len = %d, want 2", len(plan.Artifacts))
	}
}

func TestRunnerPlanRefusesUnsignedTarball(t *testing.T) {
	h := newHarness(t)
	tarball := h.buildSignedTarball(t, "0.13.0", "0.13.1")
	_ = os.Remove(tarball + ".sig")

	_, err := h.runner.Plan(context.Background(), tarball)
	if err == nil {
		t.Fatal("Plan accepted tarball with missing signature")
	}
}

func TestRunnerApplyHappyPath(t *testing.T) {
	h := newHarness(t)
	h.plantFromVersion("0.13.0")
	tarball := h.buildSignedTarball(t, "0.13.0", "0.13.1")
	h.configureSystemctlReady()
	// State file must show "ready" so health-poll exits OK on the
	// first iteration.
	h.writeStateFile("ready", 5, "0.13.1")

	res, err := h.runner.Apply(context.Background(), tarball)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.To != "0.13.1" {
		t.Fatalf("ApplyResult.To = %q, want 0.13.1", res.To)
	}
	if len(res.ArtifactsSwap) != 2 {
		t.Fatalf("ArtifactsSwap len = %d, want 2", len(res.ArtifactsSwap))
	}
	if res.JournalEndPhase != "completed" {
		t.Fatalf("JournalEndPhase = %q, want completed", res.JournalEndPhase)
	}

	// On-disk artifacts now contain the new content.
	got, _ := os.ReadFile(h.binaryPath)
	if !strings.Contains(string(got), "NEW-osvbngd") {
		t.Fatalf("on-disk osvbngd does not contain NEW content: %q", string(got))
	}

	// current-manifest.yaml records the new version.
	cur, err := CurrentInstalledVersion(context.Background(), h.stateRoot, h.binaryPath, h.cmd)
	if err != nil {
		t.Fatalf("CurrentInstalledVersion: %v", err)
	}
	if cur != "0.13.1" {
		t.Fatalf("current version = %q, want 0.13.1", cur)
	}

	// Rollback snapshot exists.
	snapDir := filepath.Join(h.runner.RollbackRoot, "0.13.0")
	if _, err := os.Stat(snapDir); err != nil {
		t.Fatalf("rollback snapshot at %s: %v", snapDir, err)
	}
}

func TestRunnerApplyRefusesTierBTarball(t *testing.T) {
	h := newHarness(t)
	tarballDir := t.TempDir()
	tarballPath := filepath.Join(tarballDir, "tierb.tar.gz")

	newOsvbngd := []byte("ignored")
	manifestYAML := fmt.Sprintf(`osvbng_version: 0.99.0
min_compatible_version: 0.0.1
type: B
build_commit: testabc
artifacts:
  - path: %s
    source: osvbngd
    sha256: %s
    mode: "0755"
`, h.binaryPath, sha256Hex(newOsvbngd))

	writeTarball(t, tarballDir, "tierb.tar.gz", []tarEntry{
		{name: "manifest.yaml", body: []byte(manifestYAML)},
		{name: "osvbngd", body: newOsvbngd, mode: 0o755},
	})
	tarballBytes, _ := os.ReadFile(tarballPath)
	digest := sha256.Sum256(tarballBytes)
	sigDER, _ := ecdsa.SignASN1(rand.Reader, h.privKey, digest[:])
	_ = os.WriteFile(tarballPath+".sig",
		[]byte(base64.StdEncoding.EncodeToString(sigDER)+"\n"), 0o644)

	h.configureSystemctlReady()
	_, err := h.runner.Apply(context.Background(), tarballPath)
	if err == nil {
		t.Fatal("Apply accepted Tier B tarball")
	}
	if !strings.Contains(err.Error(), "tier") {
		t.Fatalf("error did not mention tier: %v", err)
	}
}

func TestRunnerStatusOnFreshSystem(t *testing.T) {
	h := newHarness(t)
	// The harness stub-binary returns version via "-version" — fake
	// the Commander to satisfy CurrentInstalledVersion's fallback.
	h.cmd.scripts = []fakeResp{
		{matchName: h.binaryPath, out: "0.13.0 (testabc) built on 2026-06-15T08:00:00Z\n"},
	}

	st, err := h.runner.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.CurrentVersion != "0.13.0" {
		t.Fatalf("CurrentVersion = %q, want 0.13.0 (from osvbngd -version fallback)", st.CurrentVersion)
	}
	if st.LastUpgrade != nil {
		t.Fatalf("LastUpgrade should be nil on fresh system, got %+v", st.LastUpgrade)
	}
	if st.RollbackAvailable {
		t.Fatalf("RollbackAvailable = true on fresh system")
	}
}

func TestRunnerStatusAfterSuccessfulApply(t *testing.T) {
	h := newHarness(t)
	h.plantFromVersion("0.13.0")
	tarball := h.buildSignedTarball(t, "0.13.0", "0.13.1")
	h.configureSystemctlReady()
	h.writeStateFile("ready", 1, "0.13.1")

	if _, err := h.runner.Apply(context.Background(), tarball); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	st, err := h.runner.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.CurrentVersion != "0.13.1" {
		t.Fatalf("CurrentVersion = %q, want 0.13.1", st.CurrentVersion)
	}
	if st.LastUpgrade == nil || st.LastUpgrade.Phase != "completed" {
		t.Fatalf("LastUpgrade after Apply = %+v, want phase=completed", st.LastUpgrade)
	}
	if !st.RollbackAvailable {
		t.Fatalf("RollbackAvailable = false after successful apply")
	}
}

func TestRunnerRollbackRestoresOldContent(t *testing.T) {
	h := newHarness(t)
	h.plantFromVersion("0.13.0")
	tarball := h.buildSignedTarball(t, "0.13.0", "0.13.1")
	h.configureSystemctlReady()
	h.writeStateFile("ready", 1, "0.13.1")

	if _, err := h.runner.Apply(context.Background(), tarball); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// Confirm the new content landed.
	got, _ := os.ReadFile(h.binaryPath)
	if !strings.Contains(string(got), "NEW-osvbngd") {
		t.Fatalf("pre-rollback: expected NEW content, got %q", string(got))
	}

	// After apply the planted "ready" state matches the new (0.13.1)
	// daemon. Simulate the rolled-back (0.13.0) daemon publishing its
	// own ready state so the rollback's health-poll passes.
	h.writeStateFile("ready", 1, "0.13.0")

	rb, err := h.runner.Rollback(context.Background())
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if rb.From != "0.13.1" || rb.To != "0.13.0" {
		t.Fatalf("rb.From/To = %q/%q, want 0.13.1/0.13.0", rb.From, rb.To)
	}

	// Old content restored.
	got, _ = os.ReadFile(h.binaryPath)
	if !strings.Contains(string(got), "OLD-osvbngd") {
		t.Fatalf("rollback did not restore OLD content; got %q", string(got))
	}
}

func TestRunnerApplyRefusesWhenChainExpectedFromMismatch(t *testing.T) {
	h := newHarness(t)
	tarball := h.buildSignedTarball(t, "0.12.0", "0.13.1")
	h.configureSystemctlReady()
	h.writeStateFile("ready", 1, "0.13.1")

	// Plant a current-manifest declaring 0.99.0 — ApplyOne's
	// ExpectedFrom=0.13.0 will not match.
	planted := fmt.Sprintf(`schema_version: 2
osvbng_version: 0.99.0
min_compatible_version: 0.0.1
type: A
build_commit: x
artifacts:
  - path: %s
    source: osvbngd
    sha256: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
    mode: "0755"
    requires_restart: osvbngd
`, h.binaryPath)
	if err := os.WriteFile(filepath.Join(h.stateRoot, "current-manifest.yaml"), []byte(planted), 0o644); err != nil {
		t.Fatalf("plant current-manifest: %v", err)
	}

	_, err := h.runner.ApplyOne(context.Background(), tarball, ApplyOptions{ExpectedFrom: "0.13.0"})
	if err == nil {
		t.Fatal("ApplyOne accepted mismatched ExpectedFrom")
	}
	if !strings.Contains(err.Error(), "expected from") {
		t.Fatalf("error did not mention chain mismatch: %v", err)
	}
}

func TestRunnerNullReporterIsDefault(t *testing.T) {
	r := NewRunner(nil)
	if _, ok := r.Reporter.(NullReporter); !ok {
		t.Fatalf("NewRunner(nil).Reporter is %T, want NullReporter", r.Reporter)
	}
}

func TestRunnerApplyRestartsVPPWhenPluginSwapped(t *testing.T) {
	h := newHarness(t)
	h.plantFromVersion("0.13.0")

	pluginPath := filepath.Join(h.pluginDir, "osvbng_ipoe_plugin.so")
	if err := os.WriteFile(pluginPath, []byte("OLD-plugin"), 0o644); err != nil {
		t.Fatalf("plant old plugin: %v", err)
	}

	tarball := h.buildSignedTarballWithPlugin(t, "0.13.0", "0.13.1", pluginPath)
	h.configureSystemctlReady()
	h.writeStateFile("ready", 5, "0.13.1")

	if _, err := h.runner.Apply(context.Background(), tarball); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// vpp restart is split into stop --no-block + start --no-block (see
	// runner.restartVPP for why we no longer issue `systemctl restart`).
	// Asserting the start side is sufficient — a stop without a follow-up
	// start would leave vpp down, which isn't the intent of the plugin
	// swap path.
	sawRestart := false
	h.cmd.mu.Lock()
	defer h.cmd.mu.Unlock()
	for _, c := range h.cmd.calls {
		if c.name == "systemctl" && len(c.args) >= 3 && c.args[0] == "start" && c.args[1] == "--no-block" && c.args[2] == "vpp.service" {
			sawRestart = true
			break
		}
	}
	if !sawRestart {
		t.Fatalf("expected systemctl start --no-block vpp.service for plugin swap; saw calls: %+v", h.cmd.calls)
	}
}

func TestRunnerApplyDoesNotRestartVPPForBinaryOnlySwap(t *testing.T) {
	h := newHarness(t)
	h.plantFromVersion("0.13.0")
	tarball := h.buildSignedTarball(t, "0.13.0", "0.13.1")
	h.configureSystemctlReady()
	h.writeStateFile("ready", 5, "0.13.1")

	if _, err := h.runner.Apply(context.Background(), tarball); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// Mirror of the plugin-swap assertion: a binary-only swap must not
	// trigger the stop/start --no-block pair (the restart equivalent
	// after the wedge-mitigation refactor in runner.restartVPP).
	h.cmd.mu.Lock()
	defer h.cmd.mu.Unlock()
	for _, c := range h.cmd.calls {
		if c.name == "systemctl" && len(c.args) >= 3 && c.args[0] == "start" && c.args[1] == "--no-block" && c.args[2] == "vpp.service" {
			t.Fatalf("unexpected systemctl start --no-block vpp.service for binary-only swap; calls: %+v", h.cmd.calls)
		}
	}
}

func TestRunnerEnsureStateDirsCreatesMissingPaths(t *testing.T) {
	// Verify the helper that Apply calls at entry creates the dirs
	// it needs. The full Apply happy-path test exercises this in
	// integration; this test focuses on the helper in isolation so
	// we don't depend on a fully-wired apply flow.
	h := newHarness(t)
	_ = os.RemoveAll(h.runner.StateRoot)

	if err := h.runner.ensureStateDirs(); err != nil {
		t.Fatalf("ensureStateDirs: %v", err)
	}
	for _, dir := range []string{h.runner.StateRoot, h.runner.RollbackRoot, h.runner.QuarantineDir} {
		if _, err := os.Stat(dir); err != nil {
			t.Fatalf("ensureStateDirs did not create %s: %v", dir, err)
		}
	}
}

func TestRunnerApplyRefusesPartialApplyJournal(t *testing.T) {
	h := newHarness(t)
	h.plantFromVersion("0.13.0")
	tarball := h.buildSignedTarball(t, "0.13.0", "0.13.1")
	h.configureSystemctlReady()
	h.writeStateFile("ready", 5, "0.13.1")

	journalPath := filepath.Join(h.stateRoot, "upgrade-state.json")
	if err := os.MkdirAll(h.stateRoot, 0o755); err != nil {
		t.Fatalf("mkdir state root: %v", err)
	}
	j := NewJournal(journalPath)
	if err := j.Write(&JournalState{
		From:      "0.13.0",
		To:        "0.13.1",
		Tarball:   tarball,
		StartedAt: time.Now().UTC(),
		Phase:     "aborted_mid_swap",
	}); err != nil {
		t.Fatalf("plant partial journal: %v", err)
	}

	_, err := h.runner.Apply(context.Background(), tarball)
	if err == nil {
		t.Fatal("Apply accepted partial-apply journal without --force-retry")
	}
	if !strings.Contains(err.Error(), "non-completed state") || !strings.Contains(err.Error(), "force-retry") {
		t.Fatalf("error did not mention partial-apply guard: %v", err)
	}
}

func TestRunnerApplyForceRetryOverridesPartialApply(t *testing.T) {
	h := newHarness(t)
	h.plantFromVersion("0.13.0")
	tarball := h.buildSignedTarball(t, "0.13.0", "0.13.1")
	h.configureSystemctlReady()
	h.writeStateFile("ready", 5, "0.13.1")

	j := NewJournal(filepath.Join(h.stateRoot, "upgrade-state.json"))
	if err := os.MkdirAll(h.stateRoot, 0o755); err != nil {
		t.Fatalf("mkdir state root: %v", err)
	}
	if err := j.Write(&JournalState{
		From:      "0.13.0",
		To:        "0.13.1",
		Tarball:   tarball,
		StartedAt: time.Now().UTC(),
		Phase:     "aborted_mid_swap",
	}); err != nil {
		t.Fatalf("plant partial journal: %v", err)
	}

	res, err := h.runner.ApplyOne(context.Background(), tarball, ApplyOptions{ForceRetry: true})
	if err != nil {
		t.Fatalf("ApplyOne with ForceRetry: %v", err)
	}
	if res.JournalEndPhase != "completed" {
		t.Fatalf("JournalEndPhase = %q, want completed", res.JournalEndPhase)
	}
}

func TestRunnerApplyAcceptsCompletedJournal(t *testing.T) {
	h := newHarness(t)
	h.plantFromVersion("0.13.0")
	tarball := h.buildSignedTarball(t, "0.13.0", "0.13.1")
	h.configureSystemctlReady()
	h.writeStateFile("ready", 5, "0.13.1")

	j := NewJournal(filepath.Join(h.stateRoot, "upgrade-state.json"))
	if err := os.MkdirAll(h.stateRoot, 0o755); err != nil {
		t.Fatalf("mkdir state root: %v", err)
	}
	if err := j.Write(&JournalState{
		From:      "0.12.0",
		To:        "0.13.0",
		StartedAt: time.Now().UTC(),
		Phase:     "completed",
	}); err != nil {
		t.Fatalf("plant completed journal: %v", err)
	}

	res, err := h.runner.Apply(context.Background(), tarball)
	if err != nil {
		t.Fatalf("Apply on completed prior journal: %v", err)
	}
	if res.JournalEndPhase != "completed" {
		t.Fatalf("JournalEndPhase = %q, want completed", res.JournalEndPhase)
	}
}

// buildSignedTarballWithPrev builds a v2 tarball whose manifest
// declares previous_version + previous_manifest_sha256 and embeds the
// signed prev/manifest.yaml. Used by Phase F stepwise tests.
func (h *testHarness) buildSignedTarballWithPrev(t *testing.T, prevVersion, fromVersion, toVersion string) string {
	t.Helper()

	prevManifestYAML := fmt.Sprintf(`schema_version: 2
osvbng_version: %s
min_compatible_version: %s
type: A
build_commit: prevabc
artifacts:
  - path: %s
    source: osvbngd
    sha256: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
    mode: "0755"
    uid: -1
    gid: -1
    requires_restart: osvbngd
`, prevVersion, fromVersion, h.binaryPath)
	prevSHA := sha256Hex([]byte(prevManifestYAML))

	prevSigDigest := sha256.Sum256([]byte(prevManifestYAML))
	prevSigDER, err := ecdsa.SignASN1(rand.Reader, h.privKey, prevSigDigest[:])
	if err != nil {
		t.Fatalf("sign prev manifest: %v", err)
	}
	prevSig := []byte(base64.StdEncoding.EncodeToString(prevSigDER) + "\n")

	newOsvbngd := []byte("NEW-osvbngd-binary-" + toVersion)
	newOsvbngcli := []byte("NEW-osvbngcli-binary-" + toVersion)

	manifestYAML := fmt.Sprintf(`schema_version: 2
osvbng_version: %s
min_compatible_version: %s
previous_version: %s
previous_manifest_sha256: %s
type: A
build_commit: testabc
artifacts:
  - path: %s
    source: osvbngd
    sha256: %s
    mode: "0755"
    uid: -1
    gid: -1
    requires_restart: osvbngd
  - path: %s
    source: osvbngcli
    sha256: %s
    mode: "0755"
    uid: -1
    gid: -1
    requires_restart: none
`,
		toVersion, fromVersion, prevVersion, prevSHA,
		h.binaryPath, sha256Hex(newOsvbngd),
		h.cliPath, sha256Hex(newOsvbngcli))

	tarballDir := t.TempDir()
	tarballPath := filepath.Join(tarballDir, fmt.Sprintf("osvbng-v%s.tar.gz", toVersion))
	writeTarball(t, tarballDir, filepath.Base(tarballPath), []tarEntry{
		{name: "manifest.yaml", body: []byte(manifestYAML)},
		{name: "prev/manifest.yaml", body: []byte(prevManifestYAML)},
		{name: "prev/manifest.yaml.sig", body: prevSig},
		{name: "osvbngd", body: newOsvbngd, mode: 0o755},
		{name: "osvbngcli", body: newOsvbngcli, mode: 0o755},
	})

	tarballBytes, _ := os.ReadFile(tarballPath)
	digest := sha256.Sum256(tarballBytes)
	sigDER, err := ecdsa.SignASN1(rand.Reader, h.privKey, digest[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := os.WriteFile(tarballPath+".sig",
		[]byte(base64.StdEncoding.EncodeToString(sigDER)+"\n"), 0o644); err != nil {
		t.Fatalf("write sig: %v", err)
	}
	return tarballPath
}

func TestRunnerApplyAcceptsValidPrevManifest(t *testing.T) {
	h := newHarness(t)
	h.plantFromVersion("0.13.0")
	tarball := h.buildSignedTarballWithPrev(t, "0.13.0", "0.13.0", "0.14.0")
	h.configureSystemctlReady()
	h.writeStateFile("ready", 5, "0.14.0")

	res, err := h.runner.Apply(context.Background(), tarball)
	if err != nil {
		t.Fatalf("Apply with valid prev-manifest: %v", err)
	}
	if res.JournalEndPhase != "completed" {
		t.Fatalf("JournalEndPhase = %q, want completed", res.JournalEndPhase)
	}
}

func TestRunnerApplyRefusesPrevVersionMismatch(t *testing.T) {
	h := newHarness(t)
	// Box is at 0.13.0, but the tarball insists on jumping from 0.13.1.
	h.plantFromVersion("0.13.0")
	tarball := h.buildSignedTarballWithPrev(t, "0.13.1", "0.13.0", "0.14.0")
	h.configureSystemctlReady()
	h.writeStateFile("ready", 5, "0.14.0")

	_, err := h.runner.Apply(context.Background(), tarball)
	if err == nil {
		t.Fatal("Apply accepted stepwise mismatch")
	}
	if !strings.Contains(err.Error(), "stepwise upgrade required") {
		t.Fatalf("error did not flag stepwise mismatch: %v", err)
	}
}

func TestRunnerApplyRefusesTamperedPrevManifest(t *testing.T) {
	h := newHarness(t)
	h.plantFromVersion("0.13.0")
	tarball := h.buildSignedTarballWithPrev(t, "0.13.0", "0.13.0", "0.14.0")
	h.configureSystemctlReady()
	h.writeStateFile("ready", 5, "0.14.0")

	// Tamper with prev/manifest.yaml after the outer signature was made.
	// The outer signature won't catch this on the v1-raw scheme (it
	// signs the original tarball bytes), but the prev-manifest sha256
	// in the outer manifest must — otherwise an attacker could swap
	// prev/manifest to lie about the install lineage.
	staging, err := ExtractTarball(tarball)
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	prevPath := filepath.Join(staging.Dir, "prev", "manifest.yaml")
	tampered, _ := os.ReadFile(prevPath)
	tampered = append(tampered, []byte("\n# trailing tamper\n")...)
	if err := os.WriteFile(prevPath, tampered, 0o644); err != nil {
		t.Fatalf("write tampered prev: %v", err)
	}
	// Pack the tampered staging into a new tarball, sign it, point Apply at it.
	tampDir := t.TempDir()
	tampTarball := filepath.Join(tampDir, "tampered.tar.gz")
	writeTarball(t, tampDir, filepath.Base(tampTarball), []tarEntry{
		{name: "manifest.yaml", body: mustRead(t, filepath.Join(staging.Dir, "manifest.yaml"))},
		{name: "prev/manifest.yaml", body: tampered},
		{name: "prev/manifest.yaml.sig", body: mustRead(t, filepath.Join(staging.Dir, "prev", "manifest.yaml.sig"))},
		{name: "osvbngd", body: mustRead(t, filepath.Join(staging.Dir, "osvbngd")), mode: 0o755},
		{name: "osvbngcli", body: mustRead(t, filepath.Join(staging.Dir, "osvbngcli")), mode: 0o755},
	})
	tampBytes, _ := os.ReadFile(tampTarball)
	tampDigest := sha256.Sum256(tampBytes)
	tampSigDER, _ := ecdsa.SignASN1(rand.Reader, h.privKey, tampDigest[:])
	_ = os.WriteFile(tampTarball+".sig",
		[]byte(base64.StdEncoding.EncodeToString(tampSigDER)+"\n"), 0o644)

	_, err = h.runner.Apply(context.Background(), tampTarball)
	if err == nil {
		t.Fatal("Apply accepted tampered prev-manifest")
	}
	if !strings.Contains(err.Error(), "sha256 mismatch") && !strings.Contains(err.Error(), "signature invalid") {
		t.Fatalf("error did not flag tamper: %v", err)
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return b
}

// buildSignedFirstBootTarball builds a v2 tarball ready to be applied
// via --first-boot. Manifest declares no previous_version (this is the
// genesis tarball) and the runner sets From="first-boot".
func (h *testHarness) buildSignedFirstBootTarball(t *testing.T, toVersion string) string {
	t.Helper()
	tarballDir := t.TempDir()
	tarballPath := filepath.Join(tarballDir, fmt.Sprintf("osvbng-v%s.tar.gz", toVersion))

	newOsvbngd := []byte("NEW-osvbngd-binary-" + toVersion)
	newOsvbngcli := []byte("NEW-osvbngcli-binary-" + toVersion)

	manifestYAML := fmt.Sprintf(`schema_version: 2
osvbng_version: %s
min_compatible_version: 0.0.0
type: A
build_commit: firstboot
artifacts:
  - path: %s
    source: osvbngd
    sha256: %s
    mode: "0755"
    uid: -1
    gid: -1
    requires_restart: osvbngd
  - path: %s
    source: osvbngcli
    sha256: %s
    mode: "0755"
    uid: -1
    gid: -1
    requires_restart: none
`,
		toVersion,
		h.binaryPath, sha256Hex(newOsvbngd),
		h.cliPath, sha256Hex(newOsvbngcli))

	writeTarball(t, tarballDir, filepath.Base(tarballPath), []tarEntry{
		{name: "manifest.yaml", body: []byte(manifestYAML)},
		{name: "osvbngd", body: newOsvbngd, mode: 0o755},
		{name: "osvbngcli", body: newOsvbngcli, mode: 0o755},
	})

	tarballBytes, _ := os.ReadFile(tarballPath)
	digest := sha256.Sum256(tarballBytes)
	sigDER, err := ecdsa.SignASN1(rand.Reader, h.privKey, digest[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := os.WriteFile(tarballPath+".sig",
		[]byte(base64.StdEncoding.EncodeToString(sigDER)+"\n"), 0o644); err != nil {
		t.Fatalf("write sig: %v", err)
	}
	return tarballPath
}

func TestRunnerApplyOneFirstBootHappyPath(t *testing.T) {
	h := newHarness(t)
	// Note: NO plantFromVersion call — first-boot images have no prior
	// install. The on-disk osvbngd binary doesn't exist (the test
	// harness writes it during the swap), and there is no
	// current-manifest.yaml.
	tarball := h.buildSignedFirstBootTarball(t, "0.14.0")
	h.configureSystemctlReady()
	h.writeStateFile("ready", 1, "0.14.0")

	res, err := h.runner.ApplyOne(context.Background(), tarball, ApplyOptions{FirstBoot: true})
	if err != nil {
		t.Fatalf("ApplyOne first-boot: %v", err)
	}
	if res.From != "first-boot" {
		t.Fatalf("From = %q, want first-boot", res.From)
	}
	if res.To != "0.14.0" {
		t.Fatalf("To = %q, want 0.14.0", res.To)
	}
	if res.JournalEndPhase != "first_boot_completed" {
		t.Fatalf("JournalEndPhase = %q, want first_boot_completed", res.JournalEndPhase)
	}
	if res.SnapshotDir != "" {
		t.Fatalf("SnapshotDir = %q, must be empty on first-boot", res.SnapshotDir)
	}

	got, _ := os.ReadFile(h.binaryPath)
	if !strings.Contains(string(got), "NEW-osvbngd") {
		t.Fatalf("on-disk osvbngd does not contain NEW content: %q", string(got))
	}

	cur, err := CurrentInstalledVersion(context.Background(), h.stateRoot, h.binaryPath, h.cmd)
	if err != nil {
		t.Fatalf("CurrentInstalledVersion post-first-boot: %v", err)
	}
	if cur != "0.14.0" {
		t.Fatalf("current version = %q, want 0.14.0", cur)
	}
}

func TestRunnerApplyOneFirstBootSkipsPrevManifestVerify(t *testing.T) {
	h := newHarness(t)
	// Build a tarball that DECLARES a previous_version. A normal apply
	// would refuse this because the prev-manifest is absent; --first-boot
	// must override and proceed anyway.
	tarballDir := t.TempDir()
	tarballPath := filepath.Join(tarballDir, "osvbng-v0.14.0.tar.gz")
	newOsvbngd := []byte("NEW-osvbngd-binary-firstboot")
	newOsvbngcli := []byte("NEW-osvbngcli-binary-firstboot")
	manifestYAML := fmt.Sprintf(`schema_version: 2
osvbng_version: 0.14.0
min_compatible_version: 0.13.0
previous_version: 0.13.0
previous_manifest_sha256: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
type: A
build_commit: firstboot
artifacts:
  - path: %s
    source: osvbngd
    sha256: %s
    mode: "0755"
    uid: -1
    gid: -1
    requires_restart: osvbngd
  - path: %s
    source: osvbngcli
    sha256: %s
    mode: "0755"
    uid: -1
    gid: -1
    requires_restart: none
`, h.binaryPath, sha256Hex(newOsvbngd), h.cliPath, sha256Hex(newOsvbngcli))
	writeTarball(t, tarballDir, filepath.Base(tarballPath), []tarEntry{
		{name: "manifest.yaml", body: []byte(manifestYAML)},
		{name: "osvbngd", body: newOsvbngd, mode: 0o755},
		{name: "osvbngcli", body: newOsvbngcli, mode: 0o755},
	})
	tarballBytes, _ := os.ReadFile(tarballPath)
	digest := sha256.Sum256(tarballBytes)
	sigDER, _ := ecdsa.SignASN1(rand.Reader, h.privKey, digest[:])
	_ = os.WriteFile(tarballPath+".sig",
		[]byte(base64.StdEncoding.EncodeToString(sigDER)+"\n"), 0o644)

	h.configureSystemctlReady()
	h.writeStateFile("ready", 1, "0.14.0")

	res, err := h.runner.ApplyOne(context.Background(), tarballPath, ApplyOptions{FirstBoot: true})
	if err != nil {
		t.Fatalf("ApplyOne first-boot with declared prev-version: %v", err)
	}
	if res.JournalEndPhase != "first_boot_completed" {
		t.Fatalf("JournalEndPhase = %q, want first_boot_completed", res.JournalEndPhase)
	}
}

func TestRunnerApplyOneFirstBootRefusesNonFirstBootRetryAfterPartial(t *testing.T) {
	h := newHarness(t)
	tarball := h.buildSignedFirstBootTarball(t, "0.14.0")
	h.configureSystemctlReady()
	h.writeStateFile("ready", 1, "0.14.0")

	// Plant a partial first-boot journal.
	j := NewJournal(filepath.Join(h.stateRoot, "upgrade-state.json"))
	if err := os.MkdirAll(h.stateRoot, 0o755); err != nil {
		t.Fatalf("mkdir state root: %v", err)
	}
	if err := j.Write(&JournalState{
		From:      "first-boot",
		To:        "0.14.0",
		Tarball:   tarball,
		StartedAt: time.Now().UTC(),
		Phase:     "first_boot_swapping",
	}); err != nil {
		t.Fatalf("plant partial first-boot journal: %v", err)
	}

	// A non-firstboot retry must be refused by the partial-apply guard.
	_, err := h.runner.Apply(context.Background(), tarball)
	if err == nil {
		t.Fatal("Apply (non-first-boot) accepted a partial first-boot journal")
	}
	if !strings.Contains(err.Error(), "non-completed state") {
		t.Fatalf("error did not flag partial-apply guard: %v", err)
	}
}
