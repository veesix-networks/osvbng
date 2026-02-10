package subscriber

import (
	"context"

	subscriberComp "github.com/veesix-networks/osvbng/internal/subscriber"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/state"
	statepaths "github.com/veesix-networks/osvbng/pkg/state/paths"
)

func init() {
	show.RegisterFactory(NewStatsHandler)

	state.RegisterMetric(statepaths.SubscriberStats, paths.SubscriberStats)
}

type StatsHandler struct {
	subscriber *subscriberComp.Component
}

func NewStatsHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &StatsHandler{
		subscriber: deps.Subscriber,
	}
}

func (h *StatsHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	return h.subscriber.GetStats(ctx)
}

func (h *StatsHandler) PathPattern() paths.Path {
	return paths.SubscriberStats
}

func (h *StatsHandler) Dependencies() []paths.Path {
	return nil
}
