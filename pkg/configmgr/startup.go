package configmgr

import (
	"fmt"
	"os"

	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/config/interfaces"
)

func ensureManagementInterface(cfg *config.Config) {
	ifName := "eth0"
	if cfg.System != nil && cfg.System.ManagementInterface != "" {
		ifName = cfg.System.ManagementInterface
	}

	if cfg.Interfaces == nil {
		cfg.Interfaces = make(map[string]*interfaces.InterfaceConfig)
	}

	if _, ok := cfg.Interfaces[ifName]; ok {
		return
	}

	if _, err := os.Stat("/sys/class/net/" + ifName); os.IsNotExist(err) {
		return
	}

	cfg.Interfaces[ifName] = &interfaces.InterfaceConfig{
		Name:        ifName,
		Description: "Management Interface",
		Enabled:     true,
	}
}

func (cd *ConfigManager) LoadStartupConfig(path string) (*config.Config, error) {
	cfg, err := LoadYAML(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load startup config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("startup config validation failed: %w", err)
	}

	ensureManagementInterface(cfg)

	cd.mu.Lock()
	cd.startupConfig = cd.deepCopyConfig(cfg)
	cd.mu.Unlock()

	return cfg, nil
}

func (cd *ConfigManager) ApplyLoadedConfig() error {
	cd.mu.RLock()
	config := cd.startupConfig
	cd.mu.RUnlock()

	if config == nil {
		return fmt.Errorf("no config loaded, call LoadStartupConfig first")
	}

	cd.mu.Lock()
	cd.runningConfig = config
	cd.refreshMixedAccessSet()
	cd.refreshSGSnapshot()
	cd.mu.Unlock()

	sessionID, err := cd.CreateCandidateSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer cd.CloseCandidateSession(sessionID)

	if err := cd.LoadConfig(sessionID, config); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := cd.ProcessSubscriberGroups(sessionID, config); err != nil {
		return fmt.Errorf("failed to process subscriber groups: %w", err)
	}

	if err := cd.ProcessCGNATPools(sessionID, config); err != nil {
		return fmt.Errorf("failed to process CGNAT pools: %w", err)
	}

	versionsBefore := len(cd.versions)
	if err := cd.Commit(sessionID); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	if !cd.disableVersions && len(cd.versions) > versionsBefore {
		lastVersion := &cd.versions[len(cd.versions)-1]
		if len(cd.versions) == 1 {
			lastVersion.CommitMsg = "Initial configuration"
		} else {
			lastVersion.CommitMsg = "Startup configuration"
		}
		if err := cd.saveVersion(*lastVersion); err != nil {
			return fmt.Errorf("failed to save startup version: %w", err)
		}
	}

	return nil
}

func (cd *ConfigManager) ApplyStartupConfig(path string) error {
	_, err := cd.LoadStartupConfig(path)
	if err != nil {
		return err
	}

	return cd.ApplyLoadedConfig()
}
