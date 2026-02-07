package system

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/cppm"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

func init() {
	show.RegisterFactory(NewCPPMControlplaneHandler)
}

type CPPMControlplaneHandler struct {
	cppm *cppm.Manager
}

func NewCPPMControlplaneHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &CPPMControlplaneHandler{
		cppm: deps.CPPM,
	}
}

func (h *CPPMControlplaneHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.cppm == nil {
		return []cppm.Stats{}, nil
	}
	return h.cppm.GetStats(), nil
}

func (h *CPPMControlplaneHandler) PathPattern() paths.Path {
	return paths.SystemCPPMControlplane
}

func (h *CPPMControlplaneHandler) Dependencies() []paths.Path {
	return nil
}
