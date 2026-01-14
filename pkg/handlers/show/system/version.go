package system

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/version"
)

type VersionHandler struct {
	deps *deps.ShowDeps
}

type VersionInfo struct {
	OSVBNG         string `json:"osvbng"`
	Dataplane      string `json:"dataplane"`
	RoutingDaemons string `json:"routing_daemons"`
}

func init() {
	show.RegisterFactory(func(deps *deps.ShowDeps) show.ShowHandler {
		return &VersionHandler{deps: deps}
	})
}

func (h *VersionHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	info := &VersionInfo{
		OSVBNG: version.Full(),
	}

	if h.deps.Southbound != nil {
		if ver, err := h.deps.Southbound.GetVersion(ctx); err == nil {
			info.Dataplane = ver
		} else {
			info.Dataplane = "unavailable"
		}
	} else {
		info.Dataplane = "unavailable"
	}

	if h.deps.Routing != nil {
		if ver, err := h.deps.Routing.GetVersion(ctx); err == nil {
			info.RoutingDaemons = ver
		} else {
			info.RoutingDaemons = "unavailable"
		}
	} else {
		info.RoutingDaemons = "unavailable"
	}

	return info, nil
}

func (h *VersionHandler) PathPattern() paths.Path {
	return paths.SystemVersion
}

func (h *VersionHandler) Dependencies() []paths.Path {
	return nil
}
