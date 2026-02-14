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
