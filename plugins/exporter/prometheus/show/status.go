package show

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

func init() {
	show.RegisterFactory(NewStatusHandler)
}

type StatusHandler struct {
	pluginComponents map[string]component.Component
}

func NewStatusHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &StatusHandler{
		pluginComponents: deps.PluginComponents,
	}
}

func (h *StatusHandler) PathPattern() paths.Path {
	return paths.Path("exporters.prometheus.status")
}

type Status struct {
	State          string `json:"state"`
	ListenAddress  string `json:"listen_address,omitempty"`
	HandlerCount   int    `json:"handler_count,omitempty"`
	ServerRunning  bool   `json:"server_running,omitempty"`
	Error          string `json:"error,omitempty"`
}

type PrometheusComponent interface {
	component.Component
	Addr() string
	GetStatus() *Status
}

func (h *StatusHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	comp := h.pluginComponents["exporter.prometheus"]
	if comp == nil {
		return &Status{
			State: "not loaded",
		}, nil
	}

	promComp, ok := comp.(PrometheusComponent)
	if !ok {
		return &Status{
			State: "error",
			Error: "invalid component type",
		}, nil
	}

	return promComp.GetStatus(), nil
}

func (h *StatusHandler) Dependencies() []paths.Path {
	return nil
}
