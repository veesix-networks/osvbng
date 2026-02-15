package ldp

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewLDPEnabledHandler)
	conf.RegisterFactory(NewLDPRouterIDHandler)
	conf.RegisterFactory(NewLDPOrderedControlHandler)
	conf.RegisterFactory(NewLDPDiscoveryHelloHoldHandler)
	conf.RegisterFactory(NewLDPDiscoveryHelloIntvHandler)
	conf.RegisterFactory(NewLDPDualStackPreferIPv4Handler)
	conf.RegisterFactory(NewLDPNeighborHandler)
	conf.RegisterFactory(NewLDPIPv4Handler)
	conf.RegisterFactory(NewLDPIPv6Handler)
}

var frrReloadCallback = &conf.Callbacks{
	OnAfterApply: func(hctx *conf.HandlerContext, err error) {
		if err == nil {
			hctx.MarkFRRReloadNeeded()
		}
	},
}

type ldpHandler struct {
	path paths.Path
	deps []paths.Path
}

func (h *ldpHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ldpHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ldpHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ldpHandler) PathPattern() paths.Path {
	return h.path
}

func (h *ldpHandler) Dependencies() []paths.Path {
	return h.deps
}

func (h *ldpHandler) Callbacks() *conf.Callbacks {
	return frrReloadCallback
}

func NewLDPEnabledHandler(deps *deps.ConfDeps) conf.Handler {
	return &ldpHandler{
		path: paths.LDPEnabled,
		deps: []paths.Path{paths.MPLSEnabled},
	}
}

func NewLDPRouterIDHandler(deps *deps.ConfDeps) conf.Handler {
	return &ldpHandler{path: paths.LDPRouterID, deps: []paths.Path{paths.LDPEnabled}}
}

func NewLDPOrderedControlHandler(deps *deps.ConfDeps) conf.Handler {
	return &ldpHandler{path: paths.LDPOrderedControl, deps: []paths.Path{paths.LDPEnabled}}
}

func NewLDPDiscoveryHelloHoldHandler(deps *deps.ConfDeps) conf.Handler {
	return &ldpHandler{path: paths.LDPDiscoveryHelloHold, deps: []paths.Path{paths.LDPEnabled}}
}

func NewLDPDiscoveryHelloIntvHandler(deps *deps.ConfDeps) conf.Handler {
	return &ldpHandler{path: paths.LDPDiscoveryHelloIntv, deps: []paths.Path{paths.LDPEnabled}}
}

func NewLDPDualStackPreferIPv4Handler(deps *deps.ConfDeps) conf.Handler {
	return &ldpHandler{path: paths.LDPDualStackPreferIPv4, deps: []paths.Path{paths.LDPEnabled}}
}

func NewLDPNeighborHandler(deps *deps.ConfDeps) conf.Handler {
	return &ldpHandler{path: paths.LDPNeighbor, deps: []paths.Path{paths.LDPEnabled}}
}

func NewLDPIPv4Handler(deps *deps.ConfDeps) conf.Handler {
	return &ldpHandler{
		path: paths.LDPIPv4,
		deps: []paths.Path{paths.LDPEnabled},
	}
}

func NewLDPIPv6Handler(deps *deps.ConfDeps) conf.Handler {
	return &ldpHandler{
		path: paths.LDPIPv6,
		deps: []paths.Path{paths.LDPEnabled},
	}
}

