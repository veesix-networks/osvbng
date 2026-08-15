package vpp

import (
	"errors"
	"fmt"
	"sync"

	"go.fd.io/govpp/api"

	"github.com/veesix-networks/osvbng/pkg/config/qos"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface_types"
	cake "github.com/veesix-networks/osvbng/pkg/vpp/binapi/osvbng_qos_sched"
)

// VPP API return codes this file reacts to, from vnet/error.h. Named rather
// than inlined because -116 in a conditional says nothing about intent.
const (
	vppAPIInvalidSwIfIndex   int32 = -2
	vppAPINoSuchEntry        int32 = -6
	vppAPIEntryAlreadyExists int32 = -116
	vppAPIInstanceInUse      int32 = -147
)

// isRetval reports whether a ReceiveReply error carries this API return code.
// GoVPP turns a non-zero retval into a VPPApiError and returns it, so the
// reply's own Retval field is never reached on the paths that tolerate one.
func isRetval(err error, code int32) bool {
	var apiErr api.VPPApiError
	return errors.As(err, &apiErr) && int32(apiErr) == code
}

// The dataplane grew a second shaping tier and a weight multiplier behind new
// messages, leaving the v1 ones working and CRC-stable. A control plane can
// therefore be newer than the dataplane it is talking to, so capabilities are
// asked for rather than assumed, and the v1 path stays reachable.
//
// Deliberately not a sync.Once: the first ask can race dataplane startup, and
// caching that failure would refuse S-VLAN config until process restart. Only
// an answer latches - a reply, or a dataplane without the message.
var (
	capsMu sync.Mutex
	caps   cakeCapabilities

	// schedV2 latches whether the dataplane carries the scheduler v2 dump.
	// Discovered by sending the message rather than a capabilities bit -
	// see the comment on (*VPP).schedV2Available. Shares capsMu.
	schedV2 schedV2State
)

type schedV2State struct {
	known     bool
	supported bool
}

type cakeCapabilities struct {
	known    bool
	svlan    bool
	weighted bool
}

func (v *VPP) capabilities() cakeCapabilities {
	capsMu.Lock()
	defer capsMu.Unlock()

	if caps.known {
		return caps
	}

	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		v.logger.Warn("CAKE capabilities unavailable, will retry on next use", "error", err)
		return caps
	}
	defer ch.Close()

	reply := &cake.OsvbngCakeCapabilitiesReply{}
	if err := ch.SendRequest(&cake.OsvbngCakeCapabilities{}).ReceiveReply(reply); err != nil {
		// An older dataplane does not carry the message at all; that is
		// an answer, not a failure.
		caps = cakeCapabilities{known: true}
		v.logger.Info("CAKE capabilities not supported, using v1 messages", "error", err)
		return caps
	}

	caps = cakeCapabilities{
		known:    true,
		svlan:    reply.Features&cake.OSVBNG_CAKE_FEATURE_SVLAN_TIER != 0,
		weighted: reply.Features&cake.OSVBNG_CAKE_FEATURE_WEIGHTED_DRR != 0,
	}
	v.logger.Info("CAKE dataplane capabilities",
		"version", reply.Version, "max_levels", reply.MaxLevels,
		"svlan_tier", caps.svlan, "weighted_drr", caps.weighted)
	return caps
}

func aggregateLevel(cfg *qos.Aggregate) cake.OsvbngCakeAggLevel {
	if cfg.Level() == qos.AggregateLevelSVLAN {
		return cake.OSVBNG_CAKE_AGG_LEVEL_SVLAN
	}
	return cake.OSVBNG_CAKE_AGG_LEVEL_PORT
}

// ApplyAggregate creates the port aggregate, or one S-VLAN aggregate per
// configured tag range.
//
// A range is one dataplane object covering every tag in it, so the ranges are
// created individually rather than expanded: expanding would create one
// aggregate per tag, and the tags in a range are meant to share a shaper.
func (v *VPP) ApplyAggregate(swIfIndex uint32, cfg *qos.Aggregate) error {
	if cfg == nil {
		return nil
	}
	if !v.capabilities().svlan && cfg.Level() == qos.AggregateLevelSVLAN {
		return fmt.Errorf("dataplane has no S-VLAN aggregate tier")
	}

	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	ranges, err := cfg.ParseSVLANs()
	if err != nil {
		return err
	}
	if len(ranges) == 0 {
		ranges = []qos.SVLANRange{{}}
	}

	for _, r := range ranges {
		req := &cake.OsvbngCakeAggregateV2Create{
			SwIfIndex:       interface_types.InterfaceIndex(swIfIndex),
			Level:           aggregateLevel(cfg),
			SvlanID:         r.First,
			SvlanIDEnd:      r.Last,
			RateBytesPerSec: uint64(cfg.Rate) * 1000 / 8,
			Weight:          cfg.Weight,
			BurstNs:         cfg.BurstMs * 1_000_000,
			BufferLimit:     cfg.BufferLimit,
		}
		reply := &cake.OsvbngCakeAggregateV2CreateReply{}
		if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
			// Re-asserting what is already there is what makes checkpoint
			// replay, and applying a port before its S-VLANs, safe to repeat.
			if isRetval(err, vppAPIEntryAlreadyExists) {
				v.logger.Debug("CAKE aggregate already present",
					"sw_if_index", swIfIndex, "svlan", r.First)
				continue
			}
			return fmt.Errorf("cake aggregate create: %w", err)
		}
	}

	v.logger.Debug("Applied CAKE aggregate",
		"sw_if_index", swIfIndex, "level", cfg.Level(),
		"rate_kbps", cfg.Rate, "weight", cfg.Weight, "svlans", cfg.SVLANs)
	return nil
}

// UpdateAggregate changes rate, weight, burst or buffer limit in place.
//
// Deliberately not delete-and-recreate: that would drop the aggregate out of
// its parent's active weight and take every member's attachment and
// outstanding buffer charge with it.
func (v *VPP) UpdateAggregate(swIfIndex uint32, cfg *qos.Aggregate) error {
	if cfg == nil {
		return nil
	}

	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	ranges, err := cfg.ParseSVLANs()
	if err != nil {
		return err
	}
	if len(ranges) == 0 {
		ranges = []qos.SVLANRange{{}}
	}

	for _, r := range ranges {
		req := &cake.OsvbngCakeAggregateV2Update{
			SwIfIndex:       interface_types.InterfaceIndex(swIfIndex),
			Level:           aggregateLevel(cfg),
			SvlanID:         r.First,
			RateBytesPerSec: uint64(cfg.Rate) * 1000 / 8,
			Weight:          cfg.Weight,
			BurstNs:         cfg.BurstMs * 1_000_000,
			BufferLimit:     cfg.BufferLimit,
		}
		reply := &cake.OsvbngCakeAggregateV2UpdateReply{}
		if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
			return fmt.Errorf("cake aggregate update: %w", err)
		}
	}

	v.logger.Debug("Updated CAKE aggregate",
		"sw_if_index", swIfIndex, "rate_kbps", cfg.Rate, "weight", cfg.Weight)
	return nil
}

// RemoveAggregate deletes one tier. Deleting a port that still has S-VLANs
// beneath it is refused by the dataplane, so callers unwind children first.
func (v *VPP) RemoveAggregate(swIfIndex uint32, level uint8, svlanID uint16) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	lvl := cake.OSVBNG_CAKE_AGG_LEVEL_PORT
	if level == qos.AggregateLevelSVLAN {
		lvl = cake.OSVBNG_CAKE_AGG_LEVEL_SVLAN
	}

	req := &cake.OsvbngCakeAggregateV2Delete{
		SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
		Level:     lvl,
		SvlanID:   svlanID,
	}
	reply := &cake.OsvbngCakeAggregateV2DeleteReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		switch {
		case isRetval(err, vppAPINoSuchEntry), isRetval(err, vppAPIInvalidSwIfIndex):
			// Already gone is the outcome the caller wanted, whether the
			// aggregate went first or its interface took it along.
		case isRetval(err, vppAPIInstanceInUse):
			return fmt.Errorf("cake aggregate delete refused: S-VLANs still attached to this port")
		default:
			return fmt.Errorf("cake aggregate delete: %w", err)
		}
	}

	v.logger.Debug("Removed CAKE aggregate",
		"sw_if_index", swIfIndex, "level", level, "svlan", svlanID)
	return nil
}

// DumpAggregates reports both tiers.
func (v *VPP) DumpAggregates() ([]southbound.AggregateState, error) {
	if !v.capabilities().svlan {
		return nil, nil
	}

	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &cake.OsvbngCakeAggregateV2Dump{SwIfIndex: interface_types.InterfaceIndex(^uint32(0))}
	reqCtx := ch.SendMultiRequest(req)

	var out []southbound.AggregateState
	for {
		details := &cake.OsvbngCakeAggregateV2Details{}
		stop, err := reqCtx.ReceiveReply(details)
		if err != nil {
			return nil, fmt.Errorf("cake aggregate dump: %w", err)
		}
		if stop {
			break
		}

		level := "port"
		if details.Level == cake.OSVBNG_CAKE_AGG_LEVEL_SVLAN {
			level = "svlan"
		}

		out = append(out, southbound.AggregateState{
			SwIfIndex:        uint32(details.SwIfIndex),
			InterfaceName:    v.interfaceName(uint32(details.SwIfIndex)),
			Level:            level,
			SVLANID:          details.SvlanID,
			SVLANIDEnd:       details.SvlanIDEnd,
			RateKbps:         details.RateBytesPerSec * 8 / 1000,
			Weight:           details.Weight,
			EffectiveWeight:  details.EffectiveWeight,
			BurstMs:          details.BurstNs / 1_000_000,
			BufferUsage:      details.BufferUsage,
			BufferLimit:      details.BufferLimit,
			ActiveWeight:     details.ActiveWeight,
			ActiveChildren:   details.NActiveChildren,
			ShapedPkts:       details.ShapedPkts,
			ShapedBytes:      details.ShapedBytes,
			BackpressureEvts: details.BackpressureEvents,
			DRRBlocked:       details.DrrBlocked,
			ParentBlocked:    details.ParentBlocked,
		})
	}

	return out, nil
}
