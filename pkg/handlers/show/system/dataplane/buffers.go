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
	show.RegisterFactory(NewBuffersHandler)
	state.RegisterMetric(statepaths.SystemDataplaneBuffers, paths.SystemDataplaneBuffers)
}

type BuffersHandler struct {
	southbound southbound.Southbound
}

func NewBuffersHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BuffersHandler{southbound: deps.Southbound}
}

func (h *BuffersHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	return h.southbound.GetBufferStats()
}

func (h *BuffersHandler) PathPattern() paths.Path {
	return paths.SystemDataplaneBuffers
}

func (h *BuffersHandler) Dependencies() []paths.Path {
	return nil
}
