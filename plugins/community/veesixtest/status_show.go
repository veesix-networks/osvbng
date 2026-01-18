package veesixtest

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/state"
)

func init() {
	show.RegisterFactory(NewStatusHandler)
	state.RegisterMetric(StateStatusPath, ShowStatusPath)
}

type StatusHandler struct {
	deps *deps.ShowDeps
}

type Status struct {
	Message string `json:"message"`
	Enabled bool   `json:"enabled"`
}

func NewStatusHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &StatusHandler{deps: deps}
}

func (h *StatusHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	message := "Hello from veesixtest!"
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
