package configmgr

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

const (
	DefaultVersionDir = "/var/lib/osvbng/config-versions"
)

func (cd *ConfigManager) saveVersion(version ConfigVersion) error {
	versionDir := cd.versionDir
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		return fmt.Errorf("failed to create version directory: %w", err)
	}

	filename := filepath.Join(versionDir, fmt.Sprintf("version-%05d.yaml", version.Version))

	data, err := yaml.Marshal(version)
	if err != nil {
		return fmt.Errorf("failed to marshal version: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write version file: %w", err)
	}

	return nil
}

func (cd *ConfigManager) LoadVersions() error {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	versionDir := cd.versionDir
	if _, err := os.Stat(versionDir); os.IsNotExist(err) {
		return nil
	}

	files, err := os.ReadDir(versionDir)
	if err != nil {
		return fmt.Errorf("failed to read version directory: %w", err)
	}

	var versions []ConfigVersion
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		if filepath.Ext(file.Name()) != ".yaml" {
			continue
		}

		path := filepath.Join(versionDir, file.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read version file %s: %w", file.Name(), err)
		}

		var version ConfigVersion
		if err := yaml.Unmarshal(data, &version); err != nil {
			return fmt.Errorf("failed to parse version file %s: %w", file.Name(), err)
		}

		versions = append(versions, version)
	}

	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Version < versions[j].Version
	})

	cd.versions = versions

	return nil
}

func (cd *ConfigManager) GetVersion(version int) (*ConfigVersion, error) {
	cd.mu.RLock()
	defer cd.mu.RUnlock()

	if version < 1 || version > len(cd.versions) {
		return nil, fmt.Errorf("invalid version: %d", version)
	}

	v := cd.versions[version-1]
	return &v, nil
}

func (cd *ConfigManager) GetVersionDiff(fromVersion, toVersion int) (*DiffResult, error) {
	cd.mu.RLock()
	defer cd.mu.RUnlock()

	if fromVersion < 1 || fromVersion > len(cd.versions) {
		return nil, fmt.Errorf("invalid from version: %d", fromVersion)
	}

	if toVersion < 1 || toVersion > len(cd.versions) {
		return nil, fmt.Errorf("invalid to version: %d", toVersion)
	}

	result := &DiffResult{}

	for i := fromVersion; i < toVersion; i++ {
		v := cd.versions[i]
		for _, change := range v.Changes {
			valueStr := fmt.Sprintf("%v", change.Value)
			line := ConfigLine{
				Path:  change.Path,
				Value: valueStr,
			}
			switch change.Type {
			case "add":
				result.Added = append(result.Added, line)
			case "modify":
				result.Modified = append(result.Modified, line)
			case "delete":
				result.Deleted = append(result.Deleted, line)
			}
		}
	}

	return result, nil
}
