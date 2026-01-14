package configmgr

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/handlers/conf/types"
	"github.com/veesix-networks/osvbng/pkg/operations"
)

func TestSessionLifecycle(t *testing.T) {
	cd := NewConfigManager(operations.NewMockDataplane())

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
	cd := NewConfigManager(operations.NewMockDataplane())

	sessionID, err := cd.CreateCandidateSession()
	if err != nil {
		t.Fatalf("CreateCandidateSession failed: %v", err)
	}

	config := &types.Config{
		Interfaces: map[string]*types.InterfaceConfig{
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
	cd := NewConfigManager(operations.NewMockDataplane())
	cd.disableVersions = true

	sessionID, err := cd.CreateCandidateSession()
	if err != nil {
		t.Fatalf("CreateCandidateSession failed: %v", err)
	}

	config := &types.Config{
		Interfaces: map[string]*types.InterfaceConfig{
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
}

func TestVersionHistory(t *testing.T) {
	cd := NewConfigManager(operations.NewMockDataplane())
	cd.disableVersions = true

	sessionID, err := cd.CreateCandidateSession()
	if err != nil {
		t.Fatalf("CreateCandidateSession failed: %v", err)
	}

	config := &types.Config{
		Interfaces: map[string]*types.InterfaceConfig{
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

func TestMultipleSessions(t *testing.T) {
	cd := NewConfigManager(operations.NewMockDataplane())

	sessionID1, err := cd.CreateCandidateSession()
	if err != nil {
		t.Fatalf("CreateCandidateSession 1 failed: %v", err)
	}

	sessionID2, err := cd.CreateCandidateSession()
	if err != nil {
		t.Fatalf("CreateCandidateSession 2 failed: %v", err)
	}

	if sessionID1 == sessionID2 {
		t.Fatal("Session IDs should be unique")
	}

	if len(cd.sessions) != 2 {
		t.Fatalf("Expected 2 sessions, got %d", len(cd.sessions))
	}
}

func TestCandidateVsRunningConfig(t *testing.T) {
	cd := NewConfigManager(operations.NewMockDataplane())
	cd.disableVersions = true

	if len(cd.runningConfig.Interfaces) != 0 {
		t.Fatal("Running config should be empty initially")
	}

	sessionID, err := cd.CreateCandidateSession()
	if err != nil {
		t.Fatalf("CreateCandidateSession failed: %v", err)
	}

	config := &types.Config{
		Interfaces: map[string]*types.InterfaceConfig{
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

	cd := NewConfigManager(operations.NewMockDataplane())
	cd.disableVersions = true
	cd.startupConfigPath = filepath.Join(tmpDir, "startup-config.yaml")

	sessionID, err := cd.CreateCandidateSession()
	if err != nil {
		t.Fatalf("CreateCandidateSession failed: %v", err)
	}

	config := &types.Config{
		Interfaces: map[string]*types.InterfaceConfig{
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

	if len(cd.startupConfig.Interfaces) != 0 {
		t.Fatal("Startup config should be empty before SaveStartup")
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
	cd := NewConfigManager(operations.NewMockDataplane())
	cd.disableVersions = true

	sessionID, err := cd.CreateCandidateSession()
	if err != nil {
		t.Fatalf("CreateCandidateSession failed: %v", err)
	}

	config1 := &types.Config{
		Interfaces: map[string]*types.InterfaceConfig{
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

	config2 := &types.Config{
		Interfaces: map[string]*types.InterfaceConfig{
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
	cd := NewConfigManager(operations.NewMockDataplane())
	cd.disableVersions = true

	sessionID, err := cd.CreateCandidateSession()
	if err != nil {
		t.Fatalf("CreateCandidateSession failed: %v", err)
	}

	config := &types.Config{
		Interfaces: map[string]*types.InterfaceConfig{
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
