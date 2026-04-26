package servicegroups

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/svcgroup"
)

func init() {
	show.RegisterFactory(func(d *deps.ShowDeps) show.ShowHandler {
		return &ServiceGroupsHandler{resolver: d.SvcGroupResolver}
	})
}

type ServiceGroupsHandler struct {
	resolver *svcgroup.Resolver
}

func (h *ServiceGroupsHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	return h.resolver.GetAll(), nil
}

func (h *ServiceGroupsHandler) PathPattern() paths.Path {
	return paths.ServiceGroups
}

func (h *ServiceGroupsHandler) Dependencies() []paths.Path {
	return nil
}

func (h *ServiceGroupsHandler) Summary() string {
	return "Show all service groups"
}

func (h *ServiceGroupsHandler) Description() string {
	return "Return all configured service groups from the service group resolver."
}

func (h *ServiceGroupsHandler) SortKey() string {
	return "name"
}
