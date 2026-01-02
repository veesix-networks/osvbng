package ip

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/models/ip"
	"github.com/veesix-networks/osvbng/pkg/show/handlers"
	"github.com/veesix-networks/osvbng/pkg/show/paths"
)

type TableHandler struct {
	daemons *handlers.ShowDeps
}

func init() {
	handlers.RegisterFactory(func(daemons *handlers.ShowDeps) handlers.ShowHandler {
		return &TableHandler{daemons: daemons}
	})
}

func (h *TableHandler) PathPattern() paths.Path {
	return paths.IPTable
}

func (h *TableHandler) Dependencies() []paths.Path {
	return nil
}

func (h *TableHandler) Collect(ctx context.Context, req *handlers.ShowRequest) (interface{}, error) {
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
