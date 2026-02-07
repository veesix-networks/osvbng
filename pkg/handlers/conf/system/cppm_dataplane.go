package system

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/operations"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

func init() {
	conf.RegisterFactory(NewCPPMDataplanePolicerHandler)
}

var vppProtocolMap = map[string]uint8{
	"dhcpv4":     operations.PuntProtoDHCPv4,
	"dhcpv6":     operations.PuntProtoDHCPv6,
	"arp":        operations.PuntProtoARP,
	"pppoe-disc": operations.PuntProtoPPPoEDisc,
	"pppoe-sess": operations.PuntProtoPPPoESess,
	"ipv6-nd":    operations.PuntProtoIPv6ND,
	"l2tp":       operations.PuntProtoL2TP,
}

type CPPMDataplanePolicerHandler struct {
	southbound *southbound.VPP
}

func NewCPPMDataplanePolicerHandler(d *deps.ConfDeps) conf.Handler {
	return &CPPMDataplanePolicerHandler{
		southbound: d.Southbound,
	}
}

type DataplanePolicerConfig struct {
	Rate  float64 `json:"rate"`
	Burst uint32  `json:"burst"`
}

func (h *CPPMDataplanePolicerHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	cfg, ok := hctx.NewValue.(*DataplanePolicerConfig)
	if !ok {
		return fmt.Errorf("expected *DataplanePolicerConfig, got %T", hctx.NewValue)
	}

	if cfg.Rate < 0 {
		return fmt.Errorf("rate must be non-negative")
	}
	if cfg.Burst == 0 {
		return fmt.Errorf("burst must be greater than 0")
	}

	values, err := paths.SystemCPPMDataplanePolicer.ExtractWildcards(hctx.Path, 1)
	if err != nil {
		return fmt.Errorf("extract protocol from path: %w", err)
	}

	if _, ok := vppProtocolMap[values[0]]; !ok {
		return fmt.Errorf("unknown protocol: %s", values[0])
	}

	return nil
}

func (h *CPPMDataplanePolicerHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	cfg := hctx.NewValue.(*DataplanePolicerConfig)

	values, err := paths.SystemCPPMDataplanePolicer.ExtractWildcards(hctx.Path, 1)
	if err != nil {
		return fmt.Errorf("extract protocol from path: %w", err)
	}

	protocol := vppProtocolMap[values[0]]
	return h.southbound.ConfigurePuntPolicer(protocol, cfg.Rate, cfg.Burst)
}

func (h *CPPMDataplanePolicerHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *CPPMDataplanePolicerHandler) PathPattern() paths.Path {
	return paths.SystemCPPMDataplanePolicer
}

func (h *CPPMDataplanePolicerHandler) Dependencies() []paths.Path {
	return nil
}

func (h *CPPMDataplanePolicerHandler) Callbacks() *conf.Callbacks {
	return nil
}
