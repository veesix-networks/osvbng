package configmgr

import (
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/config"
)

func (cd *ConfigManager) LoadStartupConfig(path string) (*config.Config, error) {
	config, err := LoadYAML(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load startup config: %w", err)
	}

	cd.mu.Lock()
	cd.startupConfig = cd.deepCopyConfig(config)
	cd.mu.Unlock()

	return config, nil
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
	cd.mu.Unlock()

	sessionID, err := cd.CreateCandidateSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer cd.CloseCandidateSession(sessionID)

	if err := cd.LoadConfig(sessionID, config); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := cd.Commit(sessionID); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	if !cd.disableVersions && len(cd.versions) > 0 {
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
