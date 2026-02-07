package show

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/state"
	"github.com/veesix-networks/osvbng/plugins/community/hello"
	"github.com/veesix-networks/osvbng/plugins/exporter/prometheus/metrics"
)

func init() {
	show.RegisterFactory(NewStatusHandler)
	state.RegisterMetric(hello.StateStatusPath, hello.ShowStatusPath)
	metrics.RegisterMetricSingle[Status](hello.StateStatusPath)
}

type StatusHandler struct {
	deps *deps.ShowDeps
}

type Status struct {
	Message string `json:"message" prometheus:"label"`
	Enabled uint8  `json:"enabled" prometheus:"name=hello_plugin_enabled,help=Plugin enabled status,type=gauge"`
}

func NewStatusHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &StatusHandler{
		deps: deps,
	}
}

func (h *StatusHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	message := "Hello from example plugin!"
	var enabled uint8

	if comp, ok := h.deps.PluginComponents[hello.Namespace]; ok {
		if helloComp, ok := comp.(*hello.Component); ok {
			message = helloComp.GetMessage()
			enabled = 1
		}
	}

	return &Status{
		Message: message,
		Enabled: enabled,
	}, nil
}

func (h *StatusHandler) PathPattern() paths.Path {
	return paths.Path(hello.ShowStatusPath)
}

func (h *StatusHandler) Dependencies() []paths.Path {
	return nil
}
