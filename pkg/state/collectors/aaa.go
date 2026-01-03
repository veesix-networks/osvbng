package collectors

import (
	"context"
	"encoding/json"

	"github.com/veesix-networks/osvbng/internal/aaa"
	"github.com/veesix-networks/osvbng/pkg/state"
	statepaths "github.com/veesix-networks/osvbng/pkg/state/paths"
)

type AAAStatsProvider interface {
	GetStatsSnapshot() []*aaa.ServerStats
}

func init() {
	state.RegisterType("aaa.radius", func(provider interface{}) state.CollectorFactory {
		aaad := provider.(AAAStatsProvider)
		return func(deps *state.CollectorDeps) (state.MetricCollector, error) {
			return state.NewGenericCollector(
				"aaa.radius",
				[]string{statepaths.AAARadiusServers.String()},
				func(ctx context.Context) ([]byte, error) {
					snapshot := aaad.GetStatsSnapshot()
					return json.Marshal(snapshot)
				},
				deps.Cache,
				deps.Config,
				deps.Logger,
			), nil
		}
	})
}
