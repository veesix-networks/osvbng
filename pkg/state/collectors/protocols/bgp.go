package protocols

import (
	"context"
	"encoding/json"

	"github.com/veesix-networks/osvbng/pkg/models/protocols/bgp"
	"github.com/veesix-networks/osvbng/pkg/state"
	statepaths "github.com/veesix-networks/osvbng/pkg/state/paths"
)

type BGPStatsProvider interface {
	GetBGPStatistics(ipv4 bool) (*bgp.Statistics, error)
}

func init() {
	state.RegisterType("protocols.bgp.ipv4", func(provider interface{}) state.CollectorFactory {
		routerd := provider.(BGPStatsProvider)
		return func(deps *state.CollectorDeps) (state.MetricCollector, error) {
			return state.NewGenericCollector(
				"protocols.bgp.ipv4",
				[]string{statepaths.ProtocolsBGPStatistics.String()},
				func(ctx context.Context) ([]byte, error) {
					stats, err := routerd.GetBGPStatistics(true)
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

	state.RegisterType("protocols.bgp.ipv6", func(provider interface{}) state.CollectorFactory {
		routerd := provider.(BGPStatsProvider)
		return func(deps *state.CollectorDeps) (state.MetricCollector, error) {
			return state.NewGenericCollector(
				"protocols.bgp.ipv6",
				[]string{statepaths.ProtocolsBGPIPv6Statistics.String()},
				func(ctx context.Context) ([]byte, error) {
					stats, err := routerd.GetBGPStatistics(false)
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
