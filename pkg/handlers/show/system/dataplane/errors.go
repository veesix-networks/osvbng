package dataplane

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/state"
	statepaths "github.com/veesix-networks/osvbng/pkg/state/paths"
)

func init() {
	show.RegisterFactory(NewErrorsHandler)
	state.RegisterMetric(statepaths.SystemDataplaneErrors, paths.SystemDataplaneErrors)
}

type ErrorsHandler struct {
	southbound *southbound.VPP
}

func NewErrorsHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &ErrorsHandler{southbound: deps.Southbound}
}

func (h *ErrorsHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	return h.southbound.GetErrorStats()
}

func (h *ErrorsHandler) PathPattern() paths.Path {
	return paths.SystemDataplaneErrors
}

func (h *ErrorsHandler) Dependencies() []paths.Path {
	return nil
}
