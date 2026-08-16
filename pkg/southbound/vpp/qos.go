package vpp

// NOTE: We use the v1 name-based policer APIs (PolicerInput/PolicerOutput) instead of v2
// index-based (PolicerInputV2/PolicerOutputV2) due to an upstream VPP bug in v25.10.
// The v2 handlers send the v1 reply message ID, causing GoVPP to fail with a message
// type mismatch (expects policer_input_v2_reply but receives policer_input_reply).
// Fix in src/vnet/policer/policer_api.c:
//   line 259: REPLY_MACRO(VL_API_POLICER_INPUT_REPLY)  should be VL_API_POLICER_INPUT_V2_REPLY
//   line 293: rmp type vl_api_policer_output_reply_t    should be vl_api_policer_output_v2_reply_t
//   line 308: REPLY_MACRO(VL_API_POLICER_OUTPUT_REPLY)  should be VL_API_POLICER_OUTPUT_V2_REPLY

import (
	"fmt"

	govppapi "go.fd.io/govpp/api"

	"github.com/veesix-networks/osvbng/pkg/config/qos"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface_types"
	cake "github.com/veesix-networks/osvbng/pkg/vpp/binapi/osvbng_qos_sched"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/policer"
)

func (v *VPP) ApplyQoS(swIfIndex uint32, ingress, egress *qos.Policy) error {
	if ingress == nil && egress == nil {
		return nil
	}

	v.policerMu.Lock()
	_, exists := v.policerNames[swIfIndex]
	v.policerMu.Unlock()
	if exists {
		return nil
	}

	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	var names [2]string

	if ingress != nil {
		name := fmt.Sprintf("sub_%d_in", swIfIndex)
		cfg := ingress.ToPolicerConfig()

		addReq := &policer.PolicerAddDel{
			IsAdd:         true,
			Name:          name,
			Cir:           cfg.Cir,
			Eir:           cfg.Eir,
			Cb:            cfg.Cb,
			Eb:            cfg.Eb,
			RateType:      cfg.RateType,
			RoundType:     cfg.RoundType,
			Type:          cfg.Type,
			ColorAware:    cfg.ColorAware,
			ConformAction: cfg.ConformAction,
			ExceedAction:  cfg.ExceedAction,
			ViolateAction: cfg.ViolateAction,
		}
		addReply := &policer.PolicerAddDelReply{}
		if err := ch.SendRequest(addReq).ReceiveReply(addReply); err != nil {
			return fmt.Errorf("policer add ingress: %w", err)
		}
		if addReply.Retval != 0 {
			return fmt.Errorf("policer add ingress failed: retval=%d", addReply.Retval)
		}
		names[0] = name

		inputReq := &policer.PolicerInput{
			Name:      name,
			SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
			Apply:     true,
		}
		inputReply := &policer.PolicerInputReply{}
		if err := ch.SendRequest(inputReq).ReceiveReply(inputReply); err != nil {
			return fmt.Errorf("policer input attach: %w", err)
		}
		if inputReply.Retval != 0 {
			return fmt.Errorf("policer input attach failed: retval=%d", inputReply.Retval)
		}

		v.logger.Debug("Applied ingress policer", "sw_if_index", swIfIndex, "name", name, "cir", ingress.CIR)
	}

	if egress != nil {
		name := fmt.Sprintf("sub_%d_out", swIfIndex)
		cfg := egress.ToPolicerConfig()

		addReq := &policer.PolicerAddDel{
			IsAdd:         true,
			Name:          name,
			Cir:           cfg.Cir,
			Eir:           cfg.Eir,
			Cb:            cfg.Cb,
			Eb:            cfg.Eb,
			RateType:      cfg.RateType,
			RoundType:     cfg.RoundType,
			Type:          cfg.Type,
			ColorAware:    cfg.ColorAware,
			ConformAction: cfg.ConformAction,
			ExceedAction:  cfg.ExceedAction,
			ViolateAction: cfg.ViolateAction,
		}
		addReply := &policer.PolicerAddDelReply{}
		if err := ch.SendRequest(addReq).ReceiveReply(addReply); err != nil {
			return fmt.Errorf("policer add egress: %w", err)
		}
		if addReply.Retval != 0 {
			return fmt.Errorf("policer add egress failed: retval=%d", addReply.Retval)
		}
		names[1] = name

		outputReq := &policer.PolicerOutput{
			Name:      name,
			SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
			Apply:     true,
		}
		outputReply := &policer.PolicerOutputReply{}
		if err := ch.SendRequest(outputReq).ReceiveReply(outputReply); err != nil {
			return fmt.Errorf("policer output attach: %w", err)
		}
		if outputReply.Retval != 0 {
			return fmt.Errorf("policer output attach failed: retval=%d", outputReply.Retval)
		}

		v.logger.Debug("Applied egress policer", "sw_if_index", swIfIndex, "name", name, "cir", egress.CIR)
	}

	v.policerMu.Lock()
	v.policerNames[swIfIndex] = names
	v.policerMu.Unlock()

	return nil
}

func (v *VPP) RemoveQoS(swIfIndex uint32) error {
	v.policerMu.Lock()
	names, ok := v.policerNames[swIfIndex]
	if !ok {
		v.policerMu.Unlock()
		return nil
	}
	delete(v.policerNames, swIfIndex)
	v.policerMu.Unlock()

	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	if names[0] != "" {
		detachReq := &policer.PolicerInput{
			Name:      names[0],
			SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
			Apply:     false,
		}
		detachReply := &policer.PolicerInputReply{}
		if err := ch.SendRequest(detachReq).ReceiveReply(detachReply); err != nil {
			v.logger.Warn("Failed to detach ingress policer", "sw_if_index", swIfIndex, "error", err)
		}

		delReq := &policer.PolicerAddDel{Name: names[0], IsAdd: false}
		delReply := &policer.PolicerAddDelReply{}
		if err := ch.SendRequest(delReq).ReceiveReply(delReply); err != nil {
			v.logger.Warn("Failed to delete ingress policer", "sw_if_index", swIfIndex, "error", err)
		}
	}

	if names[1] != "" {
		detachReq := &policer.PolicerOutput{
			Name:      names[1],
			SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
			Apply:     false,
		}
		detachReply := &policer.PolicerOutputReply{}
		if err := ch.SendRequest(detachReq).ReceiveReply(detachReply); err != nil {
			v.logger.Warn("Failed to detach egress policer", "sw_if_index", swIfIndex, "error", err)
		}

		delReq := &policer.PolicerAddDel{Name: names[1], IsAdd: false}
		delReply := &policer.PolicerAddDelReply{}
		if err := ch.SendRequest(delReq).ReceiveReply(delReply); err != nil {
			v.logger.Warn("Failed to delete egress policer", "sw_if_index", swIfIndex, "error", err)
		}
	}

	v.logger.Debug("Removed QoS policers", "sw_if_index", swIfIndex)
	return nil
}

func (v *VPP) ApplyScheduler(swIfIndex uint32, rateKbps uint32, cfg *qos.SchedulerConfig) error {
	if cfg == nil || rateKbps == 0 {
		return nil
	}

	v.schedulerMu.Lock()
	if v.schedulerIfs[swIfIndex] {
		v.schedulerMu.Unlock()
		return nil
	}
	v.schedulerMu.Unlock()

	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	rateBytesPerSec := uint64(rateKbps) * 1000 / 8

	// The weight multiplier only exists on the v2 message. Against an older
	// dataplane the subscriber still gets a share proportional to its rate,
	// which is the default the weight multiplies.
	if v.capabilities().weighted {
		req := &cake.OsvbngCakeSchedV2EnableDisable{
			SwIfIndex:       interface_types.InterfaceIndex(swIfIndex),
			IsEnable:        true,
			RateBytesPerSec: rateBytesPerSec,
			TinMode:         cake.OsvbngCakeTinMode(cfg.TinModeEnum()),
			Weight:          cfg.Weight,
		}
		err = ch.SendRequest(req).ReceiveReply(&cake.OsvbngCakeSchedV2EnableDisableReply{})
	} else {
		req := &cake.OsvbngCakeSchedEnableDisable{
			SwIfIndex:       interface_types.InterfaceIndex(swIfIndex),
			IsEnable:        true,
			RateBytesPerSec: rateBytesPerSec,
			TinMode:         cake.OsvbngCakeTinMode(cfg.TinModeEnum()),
		}
		err = ch.SendRequest(req).ReceiveReply(&cake.OsvbngCakeSchedEnableDisableReply{})
	}
	// Replay of an opdb checkpoint re-asserts what is already programmed.
	if err != nil && !isRetval(err, vppAPIEntryAlreadyExists) {
		return fmt.Errorf("cake scheduler enable: %w", err)
	}

	v.schedulerMu.Lock()
	v.schedulerIfs[swIfIndex] = true
	v.schedulerMu.Unlock()

	v.logger.Debug("Applied CAKE scheduler", "sw_if_index", swIfIndex,
		"rate_kbps", rateKbps, "tin_mode", cfg.TinMode, "weight", cfg.Weight)
	return nil
}

func (v *VPP) RemoveScheduler(swIfIndex uint32) error {
	v.schedulerMu.Lock()
	if !v.schedulerIfs[swIfIndex] {
		v.schedulerMu.Unlock()
		return nil
	}
	delete(v.schedulerIfs, swIfIndex)
	v.schedulerMu.Unlock()

	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &cake.OsvbngCakeSchedEnableDisable{
		SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
		IsEnable:  false,
	}
	reply := &cake.OsvbngCakeSchedEnableDisableReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		// Session teardown usually deletes the session interface before this
		// unwind runs, and the dataplane's own interface-delete hook removes
		// the scheduler with it. Both codes mean it is already gone, which is
		// the ordinary path on every disconnect - only a real failure should
		// look like one.
		if isRetval(err, vppAPIInvalidSwIfIndex) || isRetval(err, vppAPINoSuchEntry) {
			v.logger.Debug("CAKE scheduler already removed with its interface",
				"sw_if_index", swIfIndex)
		} else {
			v.logger.Warn("Failed to disable CAKE scheduler", "sw_if_index", swIfIndex, "error", err)
		}
	}

	v.logger.Debug("Removed CAKE scheduler", "sw_if_index", swIfIndex)
	return nil
}

// interfaceName resolves a sw_if_index to its interface name, empty when the
// interface is not (or no longer) known.
func (v *VPP) interfaceName(swIfIndex uint32) string {
	if iface := v.ifMgr.Get(swIfIndex); iface != nil {
		return iface.Name
	}
	return ""
}

// DumpSchedulers reports every subscriber scheduler, through the v2 dump
// when the dataplane carries it and the v1 dump otherwise.
func (v *VPP) DumpSchedulers() ([]southbound.SchedulerState, error) {
	return v.dumpSchedulers(^uint32(0), nil)
}

// DumpScheduler reports one scheduler, nil when the interface has none.
func (v *VPP) DumpScheduler(swIfIndex uint32) (*southbound.SchedulerState, error) {
	states, err := v.dumpSchedulers(swIfIndex, nil)
	if err != nil || len(states) == 0 {
		return nil, err
	}
	return &states[0], nil
}

type schedParentFilter struct {
	swIfIndex uint32
	level     cake.OsvbngCakeAggLevel
	svlanID   uint16
}

// DumpSchedulersByParent reports the members of one aggregate. Membership
// only exists on the v2 wire, so a v1-only dataplane gets
// ErrSchedV2Unsupported rather than a silently empty answer.
func (v *VPP) DumpSchedulersByParent(parentSwIfIndex uint32, level string, svlanID uint16) ([]southbound.SchedulerState, error) {
	if !v.schedV2Available() {
		return nil, southbound.ErrSchedV2Unsupported
	}

	lvl := cake.OSVBNG_CAKE_AGG_LEVEL_PORT
	if level == "svlan" {
		lvl = cake.OSVBNG_CAKE_AGG_LEVEL_SVLAN
	}
	return v.dumpSchedulers(^uint32(0), &schedParentFilter{
		swIfIndex: parentSwIfIndex,
		level:     lvl,
		svlanID:   svlanID,
	})
}

// The v2 scheduler dump has no capabilities feature bit: adding one would
// have changed the CRC of capabilities_reply and broken every older control
// plane. Availability is discovered the way capabilities themselves are -
// send the message, and treat its absence as an answer. Only an answer
// latches; a transport failure leaves the question open for the next call.
func (v *VPP) schedV2Available() bool {
	capsMu.Lock()
	defer capsMu.Unlock()
	return !schedV2.known || schedV2.supported
}

func latchSchedV2(supported bool) {
	capsMu.Lock()
	schedV2 = schedV2State{known: true, supported: supported}
	capsMu.Unlock()
}

func (v *VPP) dumpSchedulers(swIfIndex uint32, parent *schedParentFilter) ([]southbound.SchedulerState, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	if v.schedV2Available() {
		states, v2err := v.dumpSchedulersV2(ch, swIfIndex, parent)
		if v2err == nil {
			latchSchedV2(true)
			return states, nil
		}
		if schedV2Latched() {
			return nil, v2err
		}
		// First contact failed: distinguish "message not there" from a
		// failing dataplane by asking the v1 question. Only a v1 success
		// latches the fallback.
		states, v1err := v.dumpSchedulersV1(ch, swIfIndex)
		if v1err != nil {
			return nil, fmt.Errorf("dump schedulers: %w", v2err)
		}
		latchSchedV2(false)
		v.logger.Info("CAKE scheduler v2 dump not supported, using v1", "error", v2err)
		return states, nil
	}

	return v.dumpSchedulersV1(ch, swIfIndex)
}

func schedV2Latched() bool {
	capsMu.Lock()
	defer capsMu.Unlock()
	return schedV2.known && schedV2.supported
}

func (v *VPP) dumpSchedulersV2(ch govppapi.Channel, swIfIndex uint32, parent *schedParentFilter) ([]southbound.SchedulerState, error) {
	req := &cake.OsvbngCakeSchedV2Dump{
		SwIfIndex:       interface_types.InterfaceIndex(swIfIndex),
		ParentSwIfIndex: ^interface_types.InterfaceIndex(0),
	}
	if parent != nil {
		req.ParentSwIfIndex = interface_types.InterfaceIndex(parent.swIfIndex)
		req.ParentLevel = parent.level
		req.ParentSvlanID = parent.svlanID
	}

	var result []southbound.SchedulerState

	multi := ch.SendMultiRequest(req)
	for {
		d := &cake.OsvbngCakeSchedV2Details{}
		stop, err := multi.ReceiveReply(d)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("dump schedulers v2: %w", err)
		}

		s := southbound.SchedulerState{
			SwIfIndex:       uint32(d.SwIfIndex),
			InterfaceName:   v.interfaceName(uint32(d.SwIfIndex)),
			RateKbps:        d.RateBytesPerSec * 8 / 1000,
			TinMode:         d.TinMode.String(),
			TinCount:        d.TinCnt,
			Weight:          d.Weight,
			EffectiveWeight: d.EffectiveWeight,
			HasParent:       d.HasParent,
			DRRActive:       d.DrrActive,
			DRRDeficit:      int64(d.DrrDeficit),
			DRRBlocked:      d.DrrBlocked,
			ParentBlocked:   d.ParentBlocked,
			EnqueuedPkts:    d.EnqueuedPkts,
			EnqueuedBytes:   d.EnqueuedBytes,
			DequeuedPkts:    d.DequeuedPkts,
			DequeuedBytes:   d.DequeuedBytes,
			DroppedPkts:     d.DroppedPkts,
			QueuedBuffers:   d.QueuedBuffers,
			BufferUsage:     d.BufferUsage,
			BufferLimit:     d.BufferLimit,
			OwnerThread:     d.OwnerThread,
			OverheadBytes:   d.OverheadBytes,
			ATMMode:         d.AtmMode.String(),
			MPU:             d.Mpu,
			TargetUs:        d.TargetUs,
			IntervalUs:      d.IntervalUs,
		}
		if d.HasParent {
			s.ParentSwIfIndex = uint32(d.ParentSwIfIndex)
			s.ParentInterface = v.interfaceName(uint32(d.ParentSwIfIndex))
			s.ParentSVLANID = d.ParentSvlanID
			s.ParentLevel = "port"
			if d.ParentLevel == cake.OSVBNG_CAKE_AGG_LEVEL_SVLAN {
				s.ParentLevel = "svlan"
			}
		}

		for i := uint8(0); i < d.TinCnt && i < 8; i++ {
			s.Tins = append(s.Tins, southbound.SchedulerTinState{
				Tin:         i,
				Packets:     d.TinPackets[i],
				Bytes:       d.TinBytes[i],
				Drops:       d.TinDrops[i],
				ECNMarks:    d.TinEcnMarks[i],
				SparseFlows: d.TinSparseFlows[i],
				BulkFlows:   d.TinBulkFlows[i],
				FlowCount:   d.TinFlowCount[i],
				PeakDelayUs: d.TinPeakDelayUs[i],
				AvgDelayUs:  d.TinAvgDelayUs[i],
			})
		}

		result = append(result, s)
	}

	return result, nil
}

func (v *VPP) dumpSchedulersV1(ch govppapi.Channel, swIfIndex uint32) ([]southbound.SchedulerState, error) {
	req := &cake.OsvbngCakeSchedDump{
		SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
	}

	var result []southbound.SchedulerState

	multi := ch.SendMultiRequest(req)
	for {
		d := &cake.OsvbngCakeSchedDetails{}
		stop, err := multi.ReceiveReply(d)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("dump schedulers: %w", err)
		}

		s := southbound.SchedulerState{
			SwIfIndex:     uint32(d.SwIfIndex),
			InterfaceName: v.interfaceName(uint32(d.SwIfIndex)),
			RateKbps:      d.RateBytesPerSec * 8 / 1000,
			TinMode:       d.TinMode.String(),
			TinCount:      d.TinCnt,
			BufferUsage:   d.BufferUsage,
			BufferLimit:   d.BufferLimit,
		}

		for i := uint8(0); i < d.TinCnt && i < 8; i++ {
			s.Tins = append(s.Tins, southbound.SchedulerTinState{
				Tin:         i,
				Packets:     d.TinPackets[i],
				Bytes:       d.TinBytes[i],
				Drops:       d.TinDrops[i],
				ECNMarks:    d.TinEcnMarks[i],
				SparseFlows: d.TinSparseFlows[i],
				BulkFlows:   d.TinBulkFlows[i],
			})
		}

		result = append(result, s)
	}

	return result, nil
}
