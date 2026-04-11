package subinterfaces

import (
	"context"
	"fmt"
	"strconv"

	"github.com/veesix-networks/osvbng/pkg/config/interfaces"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/operations"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

func init() {
	conf.RegisterFactory(NewSubinterfaceHandler)
}

type SubinterfaceHandler struct {
	southbound     southbound.Southbound
	dataplaneState operations.DataplaneStateReader
}

func NewSubinterfaceHandler(d *deps.ConfDeps) conf.Handler {
	return &SubinterfaceHandler{
		southbound:     d.Southbound,
		dataplaneState: d.DataplaneState,
	}
}

func (h *SubinterfaceHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	cfg, ok := hctx.NewValue.(*interfaces.SubinterfaceConfig)
	if !ok {
		return fmt.Errorf("expected *interfaces.SubinterfaceConfig, got %T", hctx.NewValue)
	}

	if cfg.VLAN < 1 || cfg.VLAN > 4094 {
		return fmt.Errorf("sub-interface vlan must be between 1 and 4094")
	}

	if cfg.InnerVLAN != nil {
		if *cfg.InnerVLAN < 1 || *cfg.InnerVLAN > 4094 {
			return fmt.Errorf("sub-interface inner-vlan must be between 1 and 4094")
		}
	}

	if hctx.Config != nil && hctx.Config.SubscriberGroups != nil {
		values, err := paths.InterfaceSubinterface.ExtractWildcards(hctx.Path, 2)
		if err == nil && len(values) >= 1 {
			parentName := values[0]
			if parentCfg, exists := hctx.Config.Interfaces[parentName]; exists && parentCfg.BNGMode == "access" {
				groupName := hctx.Config.SubscriberGroups.FindGroupNameBySVLAN(uint16(cfg.VLAN))
				if groupName != "" {
					return fmt.Errorf("sub-interface vlan %d conflicts with subscriber group %s", cfg.VLAN, groupName)
				}
			}
		}
	}

	return nil
}

func (h *SubinterfaceHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	cfg := hctx.NewValue.(*interfaces.SubinterfaceConfig)

	values, err := paths.InterfaceSubinterface.ExtractWildcards(hctx.Path, 2)
	if err != nil {
		return fmt.Errorf("extract values from path: %w", err)
	}

	parentIf := values[0]
	subIfID, err := strconv.ParseUint(values[1], 10, 16)
	if err != nil {
		return fmt.Errorf("parse subinterface id: %w", err)
	}

	subIfName := fmt.Sprintf("%s.%d", parentIf, subIfID)
	if h.dataplaneState != nil && h.dataplaneState.IsInterfaceConfigured(subIfName) {
		return nil
	}

	params := &southbound.SubinterfaceParams{
		ParentIface:  parentIf,
		SubID:        uint16(subIfID),
		OuterVLAN:    uint16(cfg.VLAN),
		InnerVLANAny: cfg.BNG != nil,
		VLANTpid:     cfg.VLANTpid,
	}

	if cfg.InnerVLAN != nil {
		v := uint16(*cfg.InnerVLAN)
		params.InnerVLAN = &v
	}

	if cfg.MSSClamp != nil {
		params.MSSClamp = &southbound.MSSClampPolicy{
			Enabled: cfg.MSSClamp.Enabled,
			IPv4MSS: cfg.MSSClamp.IPv4MSS,
			IPv6MSS: cfg.MSSClamp.IPv6MSS,
		}
	}

	return h.southbound.CreateSubinterface(params)
}

func (h *SubinterfaceHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *SubinterfaceHandler) PathPattern() paths.Path {
	return paths.InterfaceSubinterface
}

func (h *SubinterfaceHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.Interface}
}

func (h *SubinterfaceHandler) Callbacks() *conf.Callbacks {
	return nil
}

func (h *SubinterfaceHandler) Summary() string {
	return "Sub-interface configuration"
}

func (h *SubinterfaceHandler) Description() string {
	return "Create a VLAN sub-interface with outer and optional inner VLAN tags."
}

func (h *SubinterfaceHandler) ValueType() interface{} {
	return &interfaces.SubinterfaceConfig{}
}
