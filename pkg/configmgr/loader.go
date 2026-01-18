package configmgr

import (
	"fmt"
	"os"
	"reflect"

	"github.com/veesix-networks/osvbng/pkg/config"
	"gopkg.in/yaml.v3"
)

func LoadYAML(path string) (*config.Config, error) {
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

	var cfg config.Config
	if err := yaml.Unmarshal(configData, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	for name, ifCfg := range cfg.Interfaces {
		if ifCfg != nil {
			ifCfg.Name = name
		}
	}

	cfg.Plugins = make(map[string]interface{})

	for namespace, defaultCfg := range getAllPluginConfigDefaults() {
		cfgType := reflect.TypeOf(defaultCfg)
		if cfgType.Kind() == reflect.Ptr {
			cfgType = cfgType.Elem()
		}

		defaultConfig := reflect.New(cfgType).Interface()
		defaultVal := reflect.ValueOf(defaultCfg)
		if defaultVal.Kind() == reflect.Ptr {
			defaultVal = defaultVal.Elem()
		}
		reflect.ValueOf(defaultConfig).Elem().Set(defaultVal)

		SetPluginConfig(namespace, defaultConfig)
		cfg.Plugins[namespace] = defaultConfig
	}

	if pluginsRaw, ok := rawConfig["plugins"].(map[string]interface{}); ok {
		for namespace, pluginCfgRaw := range pluginsRaw {
			cfgType, ok := getPluginConfigType(namespace)
			if !ok {
				cfg.Plugins[namespace] = pluginCfgRaw
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
			cfg.Plugins[namespace] = typedConfig
		}
	}

	return &cfg, nil
}

func SaveYAML(path string, config *config.Config) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
