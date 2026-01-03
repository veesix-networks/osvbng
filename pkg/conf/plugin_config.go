package conf

import (
	"fmt"
	"reflect"
	"sync"
)

type PluginConfigRegistry struct {
	mu      sync.RWMutex
	types   map[string]reflect.Type
	configs map[string]interface{}
}

var pluginConfigRegistry = &PluginConfigRegistry{
	types:   make(map[string]reflect.Type),
	configs: make(map[string]interface{}),
}

func RegisterPluginConfig(namespace string, configInstance interface{}) {
	pluginConfigRegistry.mu.Lock()
	defer pluginConfigRegistry.mu.Unlock()

	t := reflect.TypeOf(configInstance)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if _, exists := pluginConfigRegistry.types[namespace]; exists {
		panic(fmt.Sprintf("plugin config already registered for namespace: %s", namespace))
	}

	pluginConfigRegistry.types[namespace] = t
}

func GetPluginConfig(namespace string) (interface{}, bool) {
	pluginConfigRegistry.mu.RLock()
	defer pluginConfigRegistry.mu.RUnlock()

	cfg, ok := pluginConfigRegistry.configs[namespace]
	return cfg, ok
}

func SetPluginConfig(namespace string, config interface{}) {
	pluginConfigRegistry.mu.Lock()
	defer pluginConfigRegistry.mu.Unlock()

	pluginConfigRegistry.configs[namespace] = config
}

func GetAllPluginConfigs() map[string]interface{} {
	pluginConfigRegistry.mu.RLock()
	defer pluginConfigRegistry.mu.RUnlock()

	configs := make(map[string]interface{})
	for k, v := range pluginConfigRegistry.configs {
		configs[k] = v
	}
	return configs
}

func getPluginConfigType(namespace string) (reflect.Type, bool) {
	pluginConfigRegistry.mu.RLock()
	defer pluginConfigRegistry.mu.RUnlock()

	t, ok := pluginConfigRegistry.types[namespace]
	return t, ok
}

func getAllPluginConfigTypes() map[string]reflect.Type {
	pluginConfigRegistry.mu.RLock()
	defer pluginConfigRegistry.mu.RUnlock()

	types := make(map[string]reflect.Type)
	for k, v := range pluginConfigRegistry.types {
		types[k] = v
	}
	return types
}
