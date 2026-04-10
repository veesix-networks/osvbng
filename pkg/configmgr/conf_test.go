package configmgr

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/config/interfaces"
	conf "github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

type noopHandler struct {
	path paths.Path
}

func (h *noopHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error { return nil }
func (h *noopHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error    { return nil }
func (h *noopHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error { return nil }
func (h *noopHandler) PathPattern() paths.Path                                       { return h.path }
func (h *noopHandler) Dependencies() []paths.Path                                    { return nil }
func (h *noopHandler) Callbacks() *conf.Callbacks                                    { return nil }

func newTestConfigManager(t *testing.T) *ConfigManager {
	cd := NewConfigManager()
	cd.registry.MustRegister(&noopHandler{path: "interfaces.<*>"})
	cd.registry.MustRegister(&noopHandler{path: "interfaces.<*>.enabled"})
	cd.startupConfigPath = filepath.Join(t.TempDir(), "startup-config.yaml")
	return cd
}

func TestSessionLifecycle(t *testing.T) {
	cd := newTestConfigManager(t)

	sessionID, err := cd.CreateCandidateSession()
	if err != nil {
		t.Fatalf("CreateCandidateSession failed: %v", err)
	}

	if sessionID == "" {
		t.Fatal("Session ID should not be empty")
	}

	err = cd.CloseCandidateSession(sessionID)
	if err != nil {
		t.Fatalf("CloseCandidateSession failed: %v", err)
	}

	err = cd.CloseCandidateSession(sessionID)
	if err == nil {
		t.Fatal("Expected error closing non-existent session")
	}
}

func TestLoadConfig(t *testing.T) {
	cd := newTestConfigManager(t)

	sessionID, err := cd.CreateCandidateSession()
	if err != nil {
		t.Fatalf("CreateCandidateSession failed: %v", err)
	}

	config := &config.Config{
		Interfaces: map[string]*interfaces.InterfaceConfig{
			"eth0": {
				Name:        "eth0",
				Description: "Test Interface",
				Enabled:     true,
			},
		},
	}

	err = cd.LoadConfig(sessionID, config)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	sess := cd.sessions[sessionID]
	if sess == nil {
		t.Fatal("Session should exist")
	}

	if len(sess.config.Interfaces) != 1 {
		t.Fatalf("Expected 1 interface, got %d", len(sess.config.Interfaces))
	}

	iface, exists := sess.config.Interfaces["eth0"]
	if !exists {
		t.Fatal("eth0 should exist in candidate config")
	}

	if iface.Description != "Test Interface" {
		t.Fatalf("Expected description 'Test Interface', got '%s'", iface.Description)
	}
}

func TestCommit(t *testing.T) {
	cd := newTestConfigManager(t)
	cd.disableVersions = true

	sessionID, err := cd.CreateCandidateSession()
	if err != nil {
		t.Fatalf("CreateCandidateSession failed: %v", err)
	}

	config := &config.Config{
		Interfaces: map[string]*interfaces.InterfaceConfig{
			"eth0": {
				Name:    "eth0",
				Enabled: true,
			},
		},
	}

	err = cd.LoadConfig(sessionID, config)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	err = cd.Commit(sessionID)
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	if len(cd.runningConfig.Interfaces) != 1 {
		t.Fatalf("Expected 1 interface in running config, got %d", len(cd.runningConfig.Interfaces))
	}

	_, exists := cd.runningConfig.Interfaces["eth0"]
	if !exists {
		t.Fatal("eth0 should exist in running config after commit")
	}

	if _, exists := cd.sessions[sessionID]; exists {
		t.Fatal("candidate session should be closed after successful commit")
	}
}

func TestVersionHistory(t *testing.T) {
	cd := newTestConfigManager(t)
	cd.disableVersions = true

	sessionID, err := cd.CreateCandidateSession()
	if err != nil {
		t.Fatalf("CreateCandidateSession failed: %v", err)
	}

	config := &config.Config{
		Interfaces: map[string]*interfaces.InterfaceConfig{
			"eth0": {
				Name:    "eth0",
				Enabled: true,
			},
		},
	}

	err = cd.LoadConfig(sessionID, config)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	err = cd.Commit(sessionID)
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	if len(cd.versions) != 1 {
		t.Fatalf("Expected 1 version in history, got %d", len(cd.versions))
	}

	version := cd.versions[0]
	if version.Version != 1 {
		t.Fatalf("Expected version 1, got %d", version.Version)
	}
}

func TestConfigLockPreventsMultipleSessions(t *testing.T) {
	cd := newTestConfigManager(t)

	sessionID1, err := cd.CreateCandidateSession()
	if err != nil {
		t.Fatalf("CreateCandidateSession 1 failed: %v", err)
	}

	_, err = cd.CreateCandidateSession()
	if err == nil {
		t.Fatal("expected second CreateCandidateSession to fail while lock is held")
	}
	if got := err.Error(); got != "configuration is locked by session "+string(sessionID1) {
		t.Fatalf("second session error = %q, want lock error", got)
	}
}

func TestCandidateVsRunningConfig(t *testing.T) {
	cd := newTestConfigManager(t)
	cd.disableVersions = true

	if len(cd.runningConfig.Interfaces) != 0 {
		t.Fatal("Running config should be empty initially")
	}

	sessionID, err := cd.CreateCandidateSession()
	if err != nil {
		t.Fatalf("CreateCandidateSession failed: %v", err)
	}

	config := &config.Config{
		Interfaces: map[string]*interfaces.InterfaceConfig{
			"eth0": {
				Name:    "eth0",
				Enabled: true,
			},
		},
	}

	err = cd.LoadConfig(sessionID, config)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	sess := cd.sessions[sessionID]
	if len(sess.config.Interfaces) != 1 {
		t.Fatal("Candidate config should have 1 interface")
	}
	if len(cd.runningConfig.Interfaces) != 0 {
		t.Fatal("Running config should still be empty before commit")
	}

	err = cd.Commit(sessionID)
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	if len(cd.runningConfig.Interfaces) != 1 {
		t.Fatal("Running config should have 1 interface after commit")
	}
}

func TestStartupConfig(t *testing.T) {
	tmpDir := t.TempDir()

	cd := newTestConfigManager(t)
	cd.disableVersions = true
	cd.startupConfigPath = filepath.Join(tmpDir, "startup-config.yaml")

	sessionID, err := cd.CreateCandidateSession()
	if err != nil {
		t.Fatalf("CreateCandidateSession failed: %v", err)
	}

	config := &config.Config{
		Interfaces: map[string]*interfaces.InterfaceConfig{
			"eth0": {
				Name:        "eth0",
				Description: "Test Interface",
				Enabled:     true,
			},
		},
	}

	err = cd.LoadConfig(sessionID, config)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	err = cd.Commit(sessionID)
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	if len(cd.startupConfig.Interfaces) != 1 {
		t.Fatal("Startup config should have 1 interface after Commit (auto-saved)")
	}

	err = cd.SaveStartup()
	if err != nil {
		t.Fatalf("SaveStartup failed: %v", err)
	}

	if len(cd.startupConfig.Interfaces) != 1 {
		t.Fatal("Startup config should have 1 interface after SaveStartup")
	}

	iface, exists := cd.startupConfig.Interfaces["eth0"]
	if !exists {
		t.Fatal("eth0 should exist in startup config")
	}
	if iface.Description != "Test Interface" {
		t.Fatalf("Expected description 'Test Interface', got '%s'", iface.Description)
	}

	if _, err := os.Stat(cd.startupConfigPath); os.IsNotExist(err) {
		t.Fatal("Startup config file should exist on disk")
	}
}

func TestVersionDiff(t *testing.T) {
	cd := newTestConfigManager(t)
	cd.disableVersions = true

	sessionID, err := cd.CreateCandidateSession()
	if err != nil {
		t.Fatalf("CreateCandidateSession failed: %v", err)
	}

	config1 := &config.Config{
		Interfaces: map[string]*interfaces.InterfaceConfig{
			"eth0": {
				Name:    "eth0",
				Enabled: true,
			},
		},
	}

	err = cd.LoadConfig(sessionID, config1)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	err = cd.Commit(sessionID)
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	if len(cd.versions) != 1 {
		t.Fatalf("Expected 1 version, got %d", len(cd.versions))
	}

	sessionID2, err := cd.CreateCandidateSession()
	if err != nil {
		t.Fatalf("CreateCandidateSession failed: %v", err)
	}

	config2 := &config.Config{
		Interfaces: map[string]*interfaces.InterfaceConfig{
			"eth0": {
				Name:    "eth0",
				Enabled: true,
			},
			"eth1": {
				Name:    "eth1",
				Enabled: true,
			},
		},
	}

	err = cd.LoadConfig(sessionID2, config2)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	err = cd.Commit(sessionID2)
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	if len(cd.versions) != 2 {
		t.Fatalf("Expected 2 versions, got %d", len(cd.versions))
	}

	v2 := cd.versions[1]
	if len(v2.Changes) == 0 {
		t.Fatal("Version 2 should have changes")
	}
}

func TestDryRun(t *testing.T) {
	cd := newTestConfigManager(t)
	cd.disableVersions = true

	sessionID, err := cd.CreateCandidateSession()
	if err != nil {
		t.Fatalf("CreateCandidateSession failed: %v", err)
	}

	config := &config.Config{
		Interfaces: map[string]*interfaces.InterfaceConfig{
			"eth0": {
				Name:        "eth0",
				Description: "Test Interface",
				Enabled:     true,
			},
		},
	}

	err = cd.LoadConfig(sessionID, config)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	diff, err := cd.DryRun(sessionID)
	if err != nil {
		t.Fatalf("DryRun failed: %v", err)
	}

	if len(diff.Added) == 0 {
		t.Fatal("DryRun should show added changes")
	}

	if len(cd.runningConfig.Interfaces) != 0 {
		t.Fatal("Running config should still be empty after DryRun")
	}
}

func TestExpiredCandidateSessionRejectedOnUse(t *testing.T) {
	cd := newTestConfigManager(t)

	sessionID, err := cd.CreateCandidateSession()
	if err != nil {
		t.Fatalf("CreateCandidateSession failed: %v", err)
	}

	sess := cd.sessions[sessionID]
	if sess == nil {
		t.Fatal("session should exist")
	}
	sess.lastActivity = time.Now().Add(-candidateSessionIdleTimeout - time.Second)

	err = cd.Set(sessionID, "interfaces.eth0.enabled", true)
	if err == nil {
		t.Fatal("expected expired session set to fail")
	}
	if got := err.Error(); got != "session "+string(sessionID)+" not found" {
		t.Fatalf("expired session error = %q, want not found", got)
	}
	if _, exists := cd.sessions[sessionID]; exists {
		t.Fatal("expired session should be removed during use")
	}
}
