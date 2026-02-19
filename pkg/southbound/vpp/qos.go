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

	"github.com/veesix-networks/osvbng/pkg/config/qos"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface_types"
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
