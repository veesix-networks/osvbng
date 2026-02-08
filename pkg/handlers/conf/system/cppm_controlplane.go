package system

import (
	"context"
	"fmt"

	syscfg "github.com/veesix-networks/osvbng/pkg/config/system"
	"github.com/veesix-networks/osvbng/pkg/cppm"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewCPPMControlplanePolicerHandler)
}

var cppmProtocolMap = map[string]cppm.Protocol{
	string(cppm.ProtocolDHCPv4): cppm.ProtocolDHCPv4,
	string(cppm.ProtocolDHCPv6): cppm.ProtocolDHCPv6,
	string(cppm.ProtocolARP):    cppm.ProtocolARP,
	string(cppm.ProtocolPPPoE):  cppm.ProtocolPPPoE,
	string(cppm.ProtocolIPv6ND): cppm.ProtocolIPv6ND,
	string(cppm.ProtocolL2TP):   cppm.ProtocolL2TP,
}

type CPPMControlplanePolicerHandler struct {
	cppm *cppm.Manager
}

func NewCPPMControlplanePolicerHandler(d *deps.ConfDeps) conf.Handler {
	return &CPPMControlplanePolicerHandler{
		cppm: d.CPPM,
	}
}

func (h *CPPMControlplanePolicerHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	cfg, ok := hctx.NewValue.(*syscfg.CPPMPolicerConfig)
	if !ok {
		return fmt.Errorf("expected *CPPMPolicerConfig, got %T", hctx.NewValue)
	}

	if cfg.Rate < 0 {
		return fmt.Errorf("rate must be non-negative")
	}
	if cfg.Burst <= 0 {
		return fmt.Errorf("burst must be greater than 0")
	}

	values, err := paths.SystemCPPMControlplanePolicer.ExtractWildcards(hctx.Path, 1)
	if err != nil {
		return fmt.Errorf("extract protocol from path: %w", err)
	}

	if _, ok := cppmProtocolMap[values[0]]; !ok {
		return fmt.Errorf("unknown protocol: %s", values[0])
	}

	return nil
}

func (h *CPPMControlplanePolicerHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	if h.cppm == nil {
		return fmt.Errorf("CPPM manager not available")
	}

	cfg := hctx.NewValue.(*syscfg.CPPMPolicerConfig)

	values, err := paths.SystemCPPMControlplanePolicer.ExtractWildcards(hctx.Path, 1)
	if err != nil {
		return fmt.Errorf("extract protocol from path: %w", err)
	}

	protocol := cppmProtocolMap[values[0]]
	h.cppm.Configure(protocol, cfg.Rate, int(cfg.Burst))
	return nil
}

func (h *CPPMControlplanePolicerHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *CPPMControlplanePolicerHandler) PathPattern() paths.Path {
	return paths.SystemCPPMControlplanePolicer
}

func (h *CPPMControlplanePolicerHandler) Dependencies() []paths.Path {
	return nil
}

func (h *CPPMControlplanePolicerHandler) Callbacks() *conf.Callbacks {
	return nil
}
