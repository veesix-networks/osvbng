package ip

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/models/ip"
)

type TableHandler struct {
	daemons *deps.ShowDeps
}

func init() {
	show.RegisterFactory(func(daemons *deps.ShowDeps) show.ShowHandler {
		return &TableHandler{daemons: daemons}
	})
}

func (h *TableHandler) PathPattern() paths.Path {
	return paths.IPTable
}

func (h *TableHandler) Dependencies() []paths.Path {
	return nil
}

func (h *TableHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.daemons.Southbound == nil {
		return nil, fmt.Errorf("southbound not available")
	}

	vppTables, err := h.daemons.Southbound.GetIPTables()
	if err != nil {
		return nil, fmt.Errorf("failed to get IP tables: %w", err)
	}

	var tables []*ip.TableInfo
	for _, t := range vppTables {
		tables = append(tables, &ip.TableInfo{
			TableID: t.TableID,
			Name:    t.Name,
			IsIPv6:  t.IsIPv6,
		})
	}

	return tables, nil
}
