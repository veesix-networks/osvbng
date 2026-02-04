package system

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/opdb"
)

type OpDBStatisticsHandler struct {
	deps *deps.ShowDeps
}

type OpDBStatistics struct {
	DHCPv4Sessions int    `json:"dhcpv4_sessions"`
	DHCPv6Sessions int    `json:"dhcpv6_sessions"`
	PPPoESessions  int    `json:"pppoe_sessions"`
	TotalEntries   int    `json:"total_entries"`
	Puts           uint64 `json:"puts"`
	Deletes        uint64 `json:"deletes"`
	Loads          uint64 `json:"loads"`
	Clears         uint64 `json:"clears"`
}

func init() {
	show.RegisterFactory(func(deps *deps.ShowDeps) show.ShowHandler {
		return &OpDBStatisticsHandler{deps: deps}
	})
}

func (h *OpDBStatisticsHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.deps.OpDB == nil {
		return &OpDBStatistics{}, nil
	}

	stats := &OpDBStatistics{}

	stats.DHCPv4Sessions, _ = h.deps.OpDB.Count(ctx, opdb.NamespaceDHCPv4Sessions)
	stats.DHCPv6Sessions, _ = h.deps.OpDB.Count(ctx, opdb.NamespaceDHCPv6Sessions)
	stats.PPPoESessions, _ = h.deps.OpDB.Count(ctx, opdb.NamespacePPPoESessions)
	stats.TotalEntries = stats.DHCPv4Sessions + stats.DHCPv6Sessions + stats.PPPoESessions

	ioStats := h.deps.OpDB.Stats()
	stats.Puts = ioStats.Puts
	stats.Deletes = ioStats.Deletes
	stats.Loads = ioStats.Loads
	stats.Clears = ioStats.Clears

	return stats, nil
}

func (h *OpDBStatisticsHandler) PathPattern() paths.Path {
	return paths.SystemOpDBStatistics
}

func (h *OpDBStatisticsHandler) Dependencies() []paths.Path {
	return nil
}
