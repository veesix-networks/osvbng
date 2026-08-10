package vpp

import (
	"fmt"

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

	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	cleanState := func(name string) {
		detachIn := &policer.PolicerInput{
			Name:      name,
			SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
			Apply:     false,
		}
		if err := ch.SendRequest(detachIn).ReceiveReply(&policer.PolicerInputReply{}); err != nil {
			v.logger.Warn("Failed to detach ingress policer", "name", name, "error", err)
		}
		detachOut := &policer.PolicerOutput{
			Name:      name,
			SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
			Apply:     false,
		}
		if err := ch.SendRequest(detachOut).ReceiveReply(&policer.PolicerOutputReply{}); err != nil {
			v.logger.Warn("Failed to detach egress policer", "name", name, "error", err)
		}
		delReq := &policer.PolicerAddDel{Name: name, IsAdd: false}
		if err := ch.SendRequest(delReq).ReceiveReply(&policer.PolicerAddDelReply{}); err != nil {
			v.logger.Warn("Failed to delete policer", "name", name, "error", err)
		}
	}

	var names [2]string

	if ingress != nil {
		name := fmt.Sprintf("sub_%d_in", swIfIndex)
		v.logger.Debug("Applying ingress policer", "name", name, "cir", ingress.CIR)
		cleanState(name)

		addReq := &policer.PolicerAdd{
			Name:  name,
			Infos: ingress.ToPolicerConfig(),
		}
		addReply := &policer.PolicerAddReply{}
		if err := ch.SendRequest(addReq).ReceiveReply(addReply); err != nil {
			return fmt.Errorf("policer add ingress: %w", err)
		}
		if addReply.Retval != 0 {
			return fmt.Errorf("policer add ingress failed: retval=%d", addReply.Retval)
		}
		names[0] = name

		inReq := &policer.PolicerInput{
			Name:      name,
			SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
			Apply:     true,
		}
		inReply := &policer.PolicerInputReply{}
		if err := ch.SendRequest(inReq).ReceiveReply(inReply); err != nil {
			return fmt.Errorf("policer input: %w", err)
		}
		if inReply.Retval != 0 {
			return fmt.Errorf("policer input failed: retval=%d", inReply.Retval)
		}
		v.logger.Debug("Applied ingress policer", "sw_if_index", swIfIndex, "name", name, "cir", ingress.CIR)
	}

	if egress != nil {
		name := fmt.Sprintf("sub_%d_out", swIfIndex)
		v.logger.Debug("Applying egress policer", "name", name, "cir", egress.CIR)
		cleanState(name)

		addReq := &policer.PolicerAdd{
			Name:  name,
			Infos: egress.ToPolicerConfig(),
		}
		addReply := &policer.PolicerAddReply{}
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

	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	disableReq := &cake.OsvbngCakeSchedEnableDisable{
		SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
		IsEnable:  false,
	}
	if err := ch.SendRequest(disableReq).ReceiveReply(&cake.OsvbngCakeSchedEnableDisableReply{}); err != nil {
		v.logger.Debug("No existing scheduler to disable before reapply", "sw_if_index", swIfIndex, "error", err)
	}

	rateBytesPerSec := uint64(rateKbps) * 1000 / 8

	req := &cake.OsvbngCakeSchedEnableDisable{
		SwIfIndex:       interface_types.InterfaceIndex(swIfIndex),
		IsEnable:        true,
		RateBytesPerSec: rateBytesPerSec,
		TinMode:         cake.OsvbngCakeTinMode(cfg.TinModeEnum()),
	}
	reply := &cake.OsvbngCakeSchedEnableDisableReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("cake scheduler enable: %w", err)
	}
	if reply.Retval != 0 {
		return fmt.Errorf("cake scheduler enable failed: retval=%d", reply.Retval)
	}

	v.schedulerMu.Lock()
	v.schedulerIfs[swIfIndex] = true
	v.schedulerMu.Unlock()

	v.logger.Debug("Applied CAKE scheduler", "sw_if_index", swIfIndex, "rate_kbps", rateKbps, "tin_mode", cfg.TinMode)
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
		v.logger.Warn("Failed to disable CAKE scheduler", "sw_if_index", swIfIndex, "error", err)
	}

	v.logger.Debug("Removed CAKE scheduler", "sw_if_index", swIfIndex)
	return nil
}

func (v *VPP) DumpSchedulers() ([]southbound.SchedulerState, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &cake.OsvbngCakeSchedDump{
		SwIfIndex: ^interface_types.InterfaceIndex(0),
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
			SwIfIndex:   uint32(d.SwIfIndex),
			RateKbps:    d.RateBytesPerSec * 8 / 1000,
			TinMode:     d.TinMode.String(),
			TinCount:    d.TinCnt,
			BufferUsage: d.BufferUsage,
			BufferLimit: d.BufferLimit,
		}

		for i := uint8(0); i < d.TinCnt && i < 8; i++ {
			s.Tins = append(s.Tins, southbound.SchedulerTinState{
				Packets:     d.TinPackets[i],
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
