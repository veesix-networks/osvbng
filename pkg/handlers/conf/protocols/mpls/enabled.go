package mpls

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound/vpp"
)

const defaultPlatformLabels = 1048575

func init() {
	conf.RegisterFactory(NewMPLSEnabledHandler)
}

type MPLSEnabledHandler struct {
	vpp *vpp.VPP
}

func NewMPLSEnabledHandler(deps *deps.ConfDeps) conf.Handler {
	return &MPLSEnabledHandler{
		vpp: deps.Southbound,
	}
}

func (h *MPLSEnabledHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	if _, ok := hctx.NewValue.(bool); !ok {
		return fmt.Errorf("expected bool, got %T", hctx.NewValue)
	}
	return nil
}

func (h *MPLSEnabledHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	enabled, _ := hctx.NewValue.(bool)
	if !enabled {
		return nil
	}

	cfg := hctx.Config
	if cfg == nil {
		return fmt.Errorf("candidate config not available")
	}

	platformLabels := uint32(defaultPlatformLabels)
	if cfg.Protocols.MPLS != nil && cfg.Protocols.MPLS.PlatformLabels > 0 {
		platformLabels = cfg.Protocols.MPLS.PlatformLabels
	}
	setPlatformLabels(platformLabels)

	if err := h.vpp.CreateMPLSTable(); err != nil {
		return fmt.Errorf("create MPLS table: %w", err)
	}

	for _, ifaceName := range collectMPLSInterfaces(cfg) {
		if swIfIndex, ok := h.vpp.GetIfMgr().GetSwIfIndex(ifaceName); ok {
			if err := h.vpp.EnableMPLS(swIfIndex); err != nil {
				return fmt.Errorf("enable MPLS on %s: %w", ifaceName, err)
			}
		}
	}

	return nil
}

func (h *MPLSEnabledHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *MPLSEnabledHandler) PathPattern() paths.Path {
	return paths.MPLSEnabled
}

func (h *MPLSEnabledHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.Interface}
}

func (h *MPLSEnabledHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}

func setPlatformLabels(labels uint32) {
	path := "/proc/sys/net/mpls/platform_labels"
	os.WriteFile(path, []byte(strconv.FormatUint(uint64(labels), 10)), 0644)
}
