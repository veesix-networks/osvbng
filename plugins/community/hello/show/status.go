package show

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/show/handlers"
	"github.com/veesix-networks/osvbng/pkg/show/paths"
	"github.com/veesix-networks/osvbng/plugins/community/hello"
)

func init() {
	handlers.RegisterFactory(NewStatusHandler)
}

type StatusHandler struct {
	deps *handlers.ShowDeps
}

type Status struct {
	Message string `json:"message"`
	Enabled bool   `json:"enabled"`
}

func NewStatusHandler(deps *handlers.ShowDeps) handlers.ShowHandler {
	return &StatusHandler{
		deps: deps,
	}
}

func (h *StatusHandler) Collect(ctx context.Context, req *handlers.ShowRequest) (interface{}, error) {
	message := "Hello from example plugin!"
	enabled := false

	if comp, ok := h.deps.PluginComponents[hello.Namespace]; ok {
		if helloComp, ok := comp.(*hello.Component); ok {
			message = helloComp.GetMessage()
			enabled = true
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
