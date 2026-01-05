package {{cookiecutter.plugin_name}}

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/show/handlers"
	"github.com/veesix-networks/osvbng/pkg/show/paths"
	"github.com/veesix-networks/osvbng/pkg/state"
)

func init() {
	handlers.RegisterFactory(NewStatusHandler)
	state.RegisterMetric(StateStatusPath, ShowStatusPath)
}

type StatusHandler struct {
	deps *handlers.ShowDeps
}

type Status struct {
	Message string `json:"message"`
	Enabled bool   `json:"enabled"`
}

func NewStatusHandler(deps *handlers.ShowDeps) handlers.ShowHandler {
	return &StatusHandler{deps: deps}
}

func (h *StatusHandler) Collect(ctx context.Context, req *handlers.ShowRequest) (interface{}, error) {
	message := "Hello from {{cookiecutter.plugin_name}}!"
	enabled := false

	if comp, ok := h.deps.PluginComponents[Namespace]; ok {
		if pluginComp, ok := comp.(*Component); ok {
			message = pluginComp.GetMessage()
			enabled = true
		}
	}

	return &Status{
		Message: message,
		Enabled: enabled,
	}, nil
}

func (h *StatusHandler) PathPattern() paths.Path {
	return paths.Path(ShowStatusPath)
}

func (h *StatusHandler) Dependencies() []paths.Path {
	return nil
}
