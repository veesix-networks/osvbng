package configmgr

import (
	"fmt"
	"os"
	"reflect"

	"github.com/veesix-networks/osvbng/pkg/handlers/conf/types"
	"gopkg.in/yaml.v3"
)

func LoadYAML(path string) (*types.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var rawConfig map[string]interface{}
	if err := yaml.Unmarshal(data, &rawConfig); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	configData, err := yaml.Marshal(rawConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to re-marshal config: %w", err)
	}

	var config types.Config
	if err := yaml.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	for name, ifCfg := range config.Interfaces {
		if ifCfg != nil {
			ifCfg.Name = name
		}
	}

	if pluginsRaw, ok := rawConfig["plugins"].(map[string]interface{}); ok {
		config.Plugins = make(map[string]interface{})
		for namespace, pluginCfgRaw := range pluginsRaw {
			cfgType, ok := getPluginConfigType(namespace)
			if !ok {
				config.Plugins[namespace] = pluginCfgRaw
				continue
			}

			pluginData, err := yaml.Marshal(pluginCfgRaw)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal plugin config for %s: %w", namespace, err)
			}

			typedConfig := reflect.New(cfgType).Interface()
			if err := yaml.Unmarshal(pluginData, typedConfig); err != nil {
				return nil, fmt.Errorf("failed to unmarshal plugin config for %s: %w", namespace, err)
			}

			SetPluginConfig(namespace, typedConfig)
			config.Plugins[namespace] = typedConfig
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
