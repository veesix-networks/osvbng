package subscribers

import (
	"context"
	"encoding/json"

	"github.com/veesix-networks/osvbng/pkg/models/subscribers"
	"github.com/veesix-networks/osvbng/pkg/state"
	statepaths "github.com/veesix-networks/osvbng/pkg/state/paths"
)

type SessionStatsProvider interface {
	GetSessionStats(ctx context.Context) ([]subscribers.Statistics, error)
}

func init() {
	state.RegisterType("subscriber.sessions", func(provider interface{}) state.CollectorFactory {
		dpd := provider.(SessionStatsProvider)
		return func(deps *state.CollectorDeps) (state.MetricCollector, error) {
			return state.NewGenericCollector(
				"subscriber.sessions",
				[]string{statepaths.SubscriberSessions.String()},
				func(ctx context.Context) ([]byte, error) {
					stats, err := dpd.GetSessionStats(ctx)
					if err != nil {
						return nil, err
					}
					return json.Marshal(stats)
				},
				deps.Cache,
				deps.Config,
				deps.Logger,
			), nil
		}
	})
}
