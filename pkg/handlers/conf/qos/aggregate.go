package qos

import (
	"context"
	"fmt"

	qoscfg "github.com/veesix-networks/osvbng/pkg/config/qos"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/operations"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

func init() {
	conf.RegisterFactory(NewAggregateHandler)
}

// AggregateHandler programs one tier of the shaping hierarchy.
//
// Both tiers are addressed by the port's sw_if_index, so this handler resolves
// the configured interface and lets the level in the config decide whether it
// is shaping the port itself or the S-VLANs beneath it.
type AggregateHandler struct {
	southbound     southbound.Southbound
	dataplaneState operations.DataplaneStateReader
}

func NewAggregateHandler(d *deps.ConfDeps) conf.Handler {
	return &AggregateHandler{
		southbound:     d.Southbound,
		dataplaneState: d.DataplaneState,
	}
}

func (h *AggregateHandler) extractName(path string) (string, error) {
	values, err := paths.QoSAggregate.ExtractWildcards(path, 1)
	if err != nil {
		return "", fmt.Errorf("extract qos-aggregate name from path: %w", err)
	}
	return values[0], nil
}

func (h *AggregateHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	name, err := h.extractName(hctx.Path)
	if err != nil {
		return err
	}

	if hctx.NewValue == nil {
		return nil
	}

	cfg, ok := hctx.NewValue.(*qoscfg.Aggregate)
	if !ok {
		return fmt.Errorf("expected *qos.Aggregate, got %T", hctx.NewValue)
	}

	cfg.Name = name
	if err := cfg.Validate(); err != nil {
		return err
	}

	// Cross-entry constraints - disjoint S-VLAN sets, a child not shaped above
	// its port - need the whole set and are checked by ValidateAggregates at
	// commit time. What is checkable here is that the interface exists, since
	// an aggregate is programmed against a sw_if_index. Nil during startup
	// config load, before the dataplane state is assembled.
	if h.dataplaneState != nil && !h.dataplaneState.IsInterfaceConfigured(cfg.Interface) {
		return fmt.Errorf("qos-aggregate %q: interface %s is not configured", name, cfg.Interface)
	}

	return nil
}

func (h *AggregateHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	name, err := h.extractName(hctx.Path)
	if err != nil {
		return err
	}

	if hctx.NewValue == nil {
		return h.remove(hctx.OldValue, name)
	}

	cfg, ok := hctx.NewValue.(*qoscfg.Aggregate)
	if !ok {
		return fmt.Errorf("expected *qos.Aggregate, got %T", hctx.NewValue)
	}
	cfg.Name = name

	iface := h.southbound.GetIfMgr().GetByName(cfg.Interface)
	if iface == nil {
		return fmt.Errorf("qos-aggregate %q: interface %s not found in the dataplane", name, cfg.Interface)
	}

	// Entries of one section commit in map order, so an S-VLAN can arrive
	// before the port it hangs off. Applying the port here first is
	// idempotent - already-exists is checkpoint-replay tolerated - and turns
	// the port's own later change into a no-op.
	if cfg.Level() == qoscfg.AggregateLevelSVLAN && hctx.Config != nil {
		for portName, port := range hctx.Config.QoSAggregates {
			if port.Interface == cfg.Interface && port.Level() == qoscfg.AggregateLevelPort {
				port.Name = portName
				if err := h.southbound.ApplyAggregate(iface.SwIfIndex, port); err != nil {
					return fmt.Errorf("qos-aggregate %q: port %q: %w", name, portName, err)
				}
				break
			}
		}
	}

	// A rate or weight change on an aggregate that already exists is an
	// update, never a delete and recreate: recreating drops it out of its
	// parent's active weight and takes every member's attachment and
	// outstanding buffer charge with it.
	if old, ok := hctx.OldValue.(*qoscfg.Aggregate); ok && old != nil {
		if sameShape(old, cfg) {
			return h.southbound.UpdateAggregate(iface.SwIfIndex, cfg)
		}
		// A shape change is a different set of dataplane objects: without
		// this teardown a dropped tag keeps shaping, and a widened range
		// reads the old object's already-exists as benign replay.
		if err := h.remove(old, name); err != nil {
			return err
		}
	}

	return h.southbound.ApplyAggregate(iface.SwIfIndex, cfg)
}

// sameShape reports whether two revisions address the same dataplane objects,
// so the difference can be applied in place. A change of interface or of the
// tag set is a different object and has to be torn down and rebuilt.
func sameShape(a, b *qoscfg.Aggregate) bool {
	if a.Interface != b.Interface || len(a.SVLANs) != len(b.SVLANs) {
		return false
	}
	for i := range a.SVLANs {
		if a.SVLANs[i] != b.SVLANs[i] {
			return false
		}
	}
	return true
}

func (h *AggregateHandler) remove(oldValue any, name string) error {
	old, ok := oldValue.(*qoscfg.Aggregate)
	if !ok || old == nil {
		return nil
	}

	iface := h.southbound.GetIfMgr().GetByName(old.Interface)
	if iface == nil {
		// The interface went away and took its aggregates with it.
		return nil
	}

	ranges, err := old.ParseSVLANs()
	if err != nil {
		return err
	}
	if len(ranges) == 0 {
		return h.southbound.RemoveAggregate(iface.SwIfIndex, qoscfg.AggregateLevelPort, 0)
	}

	for _, r := range ranges {
		if err := h.southbound.RemoveAggregate(iface.SwIfIndex, qoscfg.AggregateLevelSVLAN, r.First); err != nil {
			return err
		}
	}
	return nil
}

func (h *AggregateHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	name, err := h.extractName(hctx.Path)
	if err != nil {
		return err
	}

	if hctx.OldValue == nil {
		return h.remove(hctx.NewValue, name)
	}

	old, ok := hctx.OldValue.(*qoscfg.Aggregate)
	if !ok {
		return nil
	}
	old.Name = name

	iface := h.southbound.GetIfMgr().GetByName(old.Interface)
	if iface == nil {
		return nil
	}
	return h.southbound.ApplyAggregate(iface.SwIfIndex, old)
}

func (h *AggregateHandler) PathPattern() paths.Path {
	return paths.QoSAggregate
}

// Dependencies is nil like every other cross-section handler: the resolver
// substitutes this entry's wildcard into a declared dependency's pattern, so
// paths.Interface would demand interfaces.<aggregate-name> - there is no way
// to express "after any interface". Ordering after Interfaces comes from the
// Config struct walk order instead.
func (h *AggregateHandler) Dependencies() []paths.Path {
	return nil
}

func (h *AggregateHandler) Callbacks() *conf.Callbacks {
	return nil
}

func (h *AggregateHandler) Summary() string {
	return "QoS aggregate shaper"
}

func (h *AggregateHandler) Description() string {
	return "Shape a port, or the S-VLANs beneath it, with weighted fair queueing between children."
}

func (h *AggregateHandler) ValueType() interface{} {
	return &qoscfg.Aggregate{}
}
