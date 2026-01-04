package plugins

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/show/handlers"
	"github.com/veesix-networks/osvbng/pkg/show/paths"
)

func init() {
	handlers.RegisterFactory(NewPluginsInfoHandler)
}

type PluginsInfoHandler struct {
	pluginComponents map[string]component.Component
}

type PluginInfo struct {
	Name      string `json:"name"`
	Author    string `json:"author"`
	Version   string `json:"version"`
	Namespace string `json:"namespace"`
}

type PluginsInfo struct {
	TotalPlugins int          `json:"total_plugins"`
	Plugins      []PluginInfo `json:"plugins"`
}

func NewPluginsInfoHandler(deps *handlers.ShowDeps) handlers.ShowHandler {
	return &PluginsInfoHandler{
		pluginComponents: deps.PluginComponents,
	}
}

func (h *PluginsInfoHandler) Collect(ctx context.Context, req *handlers.ShowRequest) (interface{}, error) {
	plugins := make([]PluginInfo, 0, len(h.pluginComponents))

	for name := range h.pluginComponents {
		meta, ok := component.GetMetadata(name)
		if !ok {
			continue
		}

		plugins = append(plugins, PluginInfo{
			Name:      meta.Name,
			Author:    meta.Author,
			Version:   meta.Version,
			Namespace: meta.Namespace,
		})
	}

	stats := &PluginsInfo{
		TotalPlugins: len(plugins),
		Plugins:      plugins,
	}

	return stats, nil
}

func (h *PluginsInfoHandler) PathPattern() paths.Path {
	return paths.PluginsInfo
}

func (h *PluginsInfoHandler) Dependencies() []paths.Path {
	return nil
}
