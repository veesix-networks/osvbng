package show

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/telemetry"
	"github.com/veesix-networks/osvbng/plugins/community/hello"
)

func init() {
	show.RegisterFactory(NewStatusHandler)
	telemetry.RegisterMetric[Status](hello.ShowStatusPath)
}

type StatusHandler struct {
	deps *deps.ShowDeps
}

type Status struct {
	Message string `json:"message" metric:"label"`
	Enabled uint8  `json:"enabled" metric:"name=hello_plugin.enabled,type=gauge,help=Hello plugin enabled status."`
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

func (h *StatusHandler) Summary() string {
	return "Show hello plugin status"
}

func (h *StatusHandler) Description() string {
	return "Display the hello community plugin status including its configured message."
}
