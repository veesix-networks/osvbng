package subscriber

import (
	"context"

	subscriberComp "github.com/veesix-networks/osvbng/internal/subscriber"
	"github.com/veesix-networks/osvbng/pkg/show/handlers"
	"github.com/veesix-networks/osvbng/pkg/show/paths"
)

func init() {
	handlers.RegisterFactory(NewStatsHandler)
}

type StatsHandler struct {
	subscriber *subscriberComp.Component
}

func NewStatsHandler(deps *handlers.ShowDeps) handlers.ShowHandler {
	return &StatsHandler{
		subscriber: deps.Subscriber,
	}
}

func (h *StatsHandler) Collect(ctx context.Context, req *handlers.ShowRequest) (interface{}, error) {
	return h.subscriber.GetStats(ctx)
}

func (h *StatsHandler) PathPattern() paths.Path {
	return paths.SubscriberStats
}

func (h *StatsHandler) Dependencies() []paths.Path {
	return nil
}
