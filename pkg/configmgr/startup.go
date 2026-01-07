package configmgr

import (
	"fmt"
)

func (cd *ConfigManager) ApplyStartupConfig(path string) error {
	config, err := LoadYAML(path)
	if err != nil {
		return fmt.Errorf("failed to load startup config: %w", err)
	}

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

	cd.mu.Lock()
	cd.startupConfig = cd.deepCopyConfig(config)
	cd.mu.Unlock()

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
