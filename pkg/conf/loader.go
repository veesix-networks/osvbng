package conf

import (
	"fmt"
	"os"

	"github.com/veesix-networks/osvbng/pkg/conf/types"
	"gopkg.in/yaml.v3"
)

func LoadYAML(path string) (*types.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config types.Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	for name, ifCfg := range config.Interfaces {
		if ifCfg != nil {
			ifCfg.Name = name
		}
	}

	return &config, nil
}

func SaveYAML(path string, config *types.Config) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
