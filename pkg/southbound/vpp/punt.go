package vpp

import (
	"fmt"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/osvbng_punt"
	interfaces "github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface"
)

func (v *VPP) EnableARPPunt(ifaceName string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	idx, err := v.GetInterfaceIndex(ifaceName)
	if err != nil {
		return fmt.Errorf("get interface index: %w", err)
	}

	req := &osvbng_punt.OsvbngPuntEnableDisable{
		SwIfIndex: interface_types.InterfaceIndex(idx),
		Protocol:  2,
		Enable:    true,
	}

	reply := &osvbng_punt.OsvbngPuntEnableDisableReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("enable arp punt: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("enable arp punt failed: retval=%d", reply.Retval)
	}

	v.logger.Debug("Enabled ARP punt", "interface", ifaceName, "sw_if_index", idx)
	return nil
}


func (v *VPP) EnableDHCPv4Punt(ifaceName string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	idx, err := v.GetInterfaceIndex(ifaceName)
	if err != nil {
		return fmt.Errorf("get interface index: %w", err)
	}

	req := &osvbng_punt.OsvbngPuntEnableDisable{
		SwIfIndex: interface_types.InterfaceIndex(idx),
		Protocol:  0,
		Enable:    true,
	}

	reply := &osvbng_punt.OsvbngPuntEnableDisableReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("enable dhcp punt: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("enable dhcp punt failed: retval=%d", reply.Retval)
	}

	v.logger.Debug("Enabled DHCP punt", "interface", ifaceName, "sw_if_index", idx)
	return nil
}


func (v *VPP) EnableDHCPv6Punt(ifaceName string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	idx, err := v.GetInterfaceIndex(ifaceName)
	if err != nil {
		return fmt.Errorf("get interface index: %w", err)
	}

	req := &osvbng_punt.OsvbngPuntEnableDisable{
		SwIfIndex: interface_types.InterfaceIndex(idx),
		Protocol:  1, // DHCPv6
		Enable:    true,
	}

	reply := &osvbng_punt.OsvbngPuntEnableDisableReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("enable dhcpv6 punt: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("enable dhcpv6 punt failed: retval=%d", reply.Retval)
	}

	v.logger.Debug("Enabled DHCPv6 punt", "interface", ifaceName, "sw_if_index", idx)
	return nil
}


func (v *VPP) EnableIPv6NDPunt(ifaceName string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	idx, err := v.GetInterfaceIndex(ifaceName)
	if err != nil {
		return fmt.Errorf("get interface index: %w", err)
	}

	req := &osvbng_punt.OsvbngPuntEnableDisable{
		SwIfIndex: interface_types.InterfaceIndex(idx),
		Protocol:  5, // IPv6 ND
		Enable:    true,
	}

	reply := &osvbng_punt.OsvbngPuntEnableDisableReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("enable ipv6 nd punt: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("enable ipv6 nd punt failed: retval=%d", reply.Retval)
	}

	v.logger.Debug("Enabled IPv6 ND punt", "interface", ifaceName, "sw_if_index", idx)
	return nil
}


func (v *VPP) EnableL2TPPunt(ifaceName string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	idx, err := v.GetInterfaceIndex(ifaceName)
	if err != nil {
		return fmt.Errorf("get interface index: %w", err)
	}

	req := &osvbng_punt.OsvbngPuntEnableDisable{
		SwIfIndex: interface_types.InterfaceIndex(idx),
		Protocol:  6, // L2TP
		Enable:    true,
	}

	reply := &osvbng_punt.OsvbngPuntEnableDisableReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("enable l2tp punt: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("enable l2tp punt failed: retval=%d", reply.Retval)
	}

	v.logger.Debug("Enabled L2TP punt", "interface", ifaceName, "sw_if_index", idx)
	return nil
}


func (v *VPP) EnablePPPoEPunt(ifaceName string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	idx, err := v.GetInterfaceIndex(ifaceName)
	if err != nil {
		return fmt.Errorf("get interface index: %w", err)
	}

	for _, protocol := range []uint8{3, 4} {
		req := &osvbng_punt.OsvbngPuntEnableDisable{
			SwIfIndex: interface_types.InterfaceIndex(idx),
			Protocol:  protocol,
			Enable:    true,
		}

		reply := &osvbng_punt.OsvbngPuntEnableDisableReply{}
		if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
			return fmt.Errorf("enable pppoe punt (protocol %d): %w", protocol, err)
		}

		if reply.Retval != 0 {
			return fmt.Errorf("enable pppoe punt (protocol %d) failed: retval=%d", protocol, reply.Retval)
		}
	}

	v.logger.Debug("Enabled PPPoE punt", "interface", ifaceName, "sw_if_index", idx)
	return nil
}


func (v *VPP) DisableARPReply(ifaceName string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	idx, err := v.GetInterfaceIndex(ifaceName)
	if err != nil {
		return fmt.Errorf("get interface index: %w", err)
	}
	swIfIndex := interface_types.InterfaceIndex(idx)

	req := &interfaces.SwInterfaceSetFlags{
		SwIfIndex: swIfIndex,
		Flags:     interface_types.IF_STATUS_API_FLAG_ADMIN_UP,
	}

	reply := &interfaces.SwInterfaceSetFlagsReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("set interface flags: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("disable arp-reply failed: retval=%d", reply.Retval)
	}

	v.logger.Debug("Disabled ARP reply", "interface", ifaceName, "sw_if_index", idx)
	return nil
}


func (v *VPP) DumpPuntRegistrations() ([]southbound.PuntRegistration, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, err
	}
	defer ch.Close()

	req := &osvbng_punt.OsvbngPuntRegistrationDump{}

	stream := ch.SendMultiRequest(req)
	var result []southbound.PuntRegistration

	for {
		reply := &osvbng_punt.OsvbngPuntRegistrationDetails{}
		stop, err := stream.ReceiveReply(reply)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("dump punt registrations: %w", err)
		}

		info := southbound.PuntRegistration{
			SwIfIndex: uint32(reply.SwIfIndex),
			Protocol:  reply.Protocol,
		}
		result = append(result, info)
	}

	v.logger.Debug("Dumped punt registrations", "count", len(result))
	return result, nil
}


func (v *VPP) GetPuntStats() ([]southbound.PuntStats, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, err
	}
	defer ch.Close()

	req := &osvbng_punt.OsvbngPuntStatsDump{}
	stream := ch.SendMultiRequest(req)
	var result []southbound.PuntStats

	for {
		reply := &osvbng_punt.OsvbngPuntStatsDetails{}
		stop, err := stream.ReceiveReply(reply)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("dump punt stats: %w", err)
		}

		stats := southbound.PuntStats{
			Protocol:       reply.Protocol,
			PacketsPunted:  reply.PacketsPunted,
			PacketsDropped: reply.PacketsDropped,
			PacketsPoliced: reply.PacketsPoliced,
			PolicerRate:    reply.PolicerRate,
			PolicerBurst:   reply.PolicerBurst,
		}
		result = append(result, stats)
	}

	return result, nil
}


func (v *VPP) ConfigurePuntPolicer(protocol uint8, rate float64, burst uint32) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return err
	}
	defer ch.Close()

	req := &osvbng_punt.OsvbngPuntPolicerConfigure{
		Protocol: protocol,
		Rate:     rate,
		Burst:    burst,
	}

	reply := &osvbng_punt.OsvbngPuntPolicerConfigureReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("configure punt policer: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("configure punt policer failed: retval=%d", reply.Retval)
	}

	v.logger.Debug("Configured punt policer", "protocol", protocol, "rate", rate, "burst", burst)
	return nil
}


