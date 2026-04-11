package vpp

import (
	"fmt"
	"net"

	"github.com/veesix-networks/osvbng/pkg/ifmgr"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ethernet_types"
	vppinterfaces "github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/osvbng_pppoe"
	"go.fd.io/govpp/api"
)

func (v *VPP) AddPPPoESession(sessionID uint16, clientIP net.IP, clientMAC net.HardwareAddr, localMAC net.HardwareAddr, encapIfIndex uint32, outerVLAN uint16, innerVLAN uint16, decapVrfID uint32, pppMTU uint16, policy southbound.MSSClampPolicy) (uint32, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return 0, fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	clientAddr, err := v.toAddress(clientIP)
	if err != nil {
		return 0, fmt.Errorf("convert client IP: %w", err)
	}

	var clientMacAddr ethernet_types.MacAddress
	copy(clientMacAddr[:], clientMAC)

	var localMacAddr ethernet_types.MacAddress
	copy(localMacAddr[:], localMAC)

	req := &osvbng_pppoe.OsvbngPppoeAddDelSession{
		IsAdd:        true,
		SessionID:    sessionID,
		ClientIP:     clientAddr,
		DecapVrfID:   decapVrfID,
		ClientMac:    clientMacAddr,
		LocalMac:     localMacAddr,
		EncapIfIndex: interface_types.InterfaceIndex(encapIfIndex),
		OuterVlan:    outerVLAN,
		InnerVlan:    innerVLAN,
	}

	reply := &osvbng_pppoe.OsvbngPppoeAddDelSessionReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return 0, fmt.Errorf("add pppoe session: %w", err)
	}

	if reply.Retval != 0 {
		return 0, fmt.Errorf("add pppoe session failed: retval=%d", reply.Retval)
	}

	swIdx := uint32(reply.SwIfIndex)

	if err := v.programPPPoESessionMTUAndClamp(ch, sessionID, swIdx, pppMTU, policy, clientIP, clientMAC); err != nil {
		return 0, err
	}

	v.ifMgr.Add(&ifmgr.Interface{
		SwIfIndex:    swIdx,
		SupSwIfIndex: encapIfIndex,
		Name:         fmt.Sprintf("pppoe-session-%d", sessionID),
		Type:         ifmgr.IfTypeP2P,
		AdminUp:      true,
		FIBTableID:   decapVrfID,
		MTU:          uint32(pppMTU),
	})

	v.logger.Debug("Added PPPoE session to VPP",
		"session_id", sessionID,
		"client_ip", clientIP.String(),
		"client_mac", clientMAC.String(),
		"local_mac", localMAC.String(),
		"encap_if_index", encapIfIndex,
		"outer_vlan", outerVLAN,
		"inner_vlan", innerVLAN,
		"sw_if_index", reply.SwIfIndex,
		"ppp_mtu", pppMTU,
		"ipv4_mss", policy.IPv4MSS,
		"ipv6_mss", policy.IPv6MSS)

	return swIdx, nil
}

// programPPPoESessionMTUAndClamp sets the per-session interface MTU to the
// negotiated PPP MTU and programs the MSS clamp. On any failure it rolls back
// the just-created PPPoE session via OsvbngPppoeAddDelSession(IsAdd: false)
// so the caller can return without leaving an orphan VPP interface, and
// without ever adding the half-created session to ifMgr.
//
// Logs the rollback result distinctly from the original error so post-mortem
// analysis can tell whether a leaked interface was collected by the rollback
// or escaped to the janitor.
func (v *VPP) programPPPoESessionMTUAndClamp(ch api.Channel, sessionID uint16, swIdx uint32, pppMTU uint16, policy southbound.MSSClampPolicy, clientIP net.IP, clientMAC net.HardwareAddr) error {
	if pppMTU == 0 {
		pppMTU = 1492
	}

	mtuReq := &vppinterfaces.SwInterfaceSetMtu{
		SwIfIndex: interface_types.InterfaceIndex(swIdx),
		Mtu:       []uint32{uint32(pppMTU), 0, 0, 0},
	}
	mtuReply := &vppinterfaces.SwInterfaceSetMtuReply{}
	if err := ch.SendRequest(mtuReq).ReceiveReply(mtuReply); err != nil {
		v.rollbackPPPoESession(sessionID, swIdx, clientIP, clientMAC, fmt.Errorf("set ppp mtu: %w", err))
		return fmt.Errorf("set ppp mtu on pppoe-session-%d: %w", sessionID, err)
	}
	if mtuReply.Retval != 0 {
		v.rollbackPPPoESession(sessionID, swIdx, clientIP, clientMAC, fmt.Errorf("set ppp mtu retval=%d", mtuReply.Retval))
		return fmt.Errorf("set ppp mtu on pppoe-session-%d: retval=%d", sessionID, mtuReply.Retval)
	}

	if policy.Enabled {
		if err := v.EnableMSSClamp(swIdx, policy); err != nil {
			v.rollbackPPPoESession(sessionID, swIdx, clientIP, clientMAC, err)
			return fmt.Errorf("enable mss clamp on pppoe-session-%d: %w", sessionID, err)
		}
	}

	return nil
}

// rollbackPPPoESession deletes a half-created PPPoE session interface after a
// failure during MTU/clamp programming. Logs the rollback result distinctly so
// the operator can tell rollback-success from rollback-failure for orphan
// tracking.
func (v *VPP) rollbackPPPoESession(sessionID uint16, swIdx uint32, clientIP net.IP, clientMAC net.HardwareAddr, originalErr error) {
	clientAddr, addrErr := v.toAddress(clientIP)
	if addrErr != nil {
		v.logger.Error("PPPoE session rollback failed: bad client IP",
			"session_id", sessionID,
			"sw_if_index", swIdx,
			"original_error", originalErr,
			"rollback_error", addrErr)
		return
	}

	var clientMacAddr ethernet_types.MacAddress
	copy(clientMacAddr[:], clientMAC)

	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		v.logger.Error("PPPoE session rollback failed: cannot open channel",
			"session_id", sessionID,
			"sw_if_index", swIdx,
			"original_error", originalErr,
			"rollback_error", err)
		return
	}
	defer ch.Close()

	delReq := &osvbng_pppoe.OsvbngPppoeAddDelSession{
		IsAdd:     false,
		SessionID: sessionID,
		ClientIP:  clientAddr,
		ClientMac: clientMacAddr,
	}
	delReply := &osvbng_pppoe.OsvbngPppoeAddDelSessionReply{}
	if err := ch.SendRequest(delReq).ReceiveReply(delReply); err != nil {
		v.logger.Error("PPPoE session rollback delete failed (orphan left for janitor)",
			"session_id", sessionID,
			"sw_if_index", swIdx,
			"original_error", originalErr,
			"rollback_error", err)
		return
	}
	if delReply.Retval != 0 {
		v.logger.Error("PPPoE session rollback delete returned non-zero (orphan left for janitor)",
			"session_id", sessionID,
			"sw_if_index", swIdx,
			"original_error", originalErr,
			"rollback_retval", delReply.Retval)
		return
	}

	v.logger.Info("PPPoE session rollback delete succeeded after MTU/clamp failure",
		"session_id", sessionID,
		"sw_if_index", swIdx,
		"original_error", originalErr)
}


func (v *VPP) DeletePPPoESession(sessionID uint16, clientIP net.IP, clientMAC net.HardwareAddr) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	clientAddr, err := v.toAddress(clientIP)
	if err != nil {
		return fmt.Errorf("convert client IP: %w", err)
	}

	var clientMacAddr ethernet_types.MacAddress
	copy(clientMacAddr[:], clientMAC)

	// For delete, VPP only uses (client_mac, session_id) for bihash lookup
	// and client_ip for FIB removal. Other fields are ignored.
	req := &osvbng_pppoe.OsvbngPppoeAddDelSession{
		IsAdd:     false,
		SessionID: sessionID,
		ClientIP:  clientAddr,
		ClientMac: clientMacAddr,
	}

	reply := &osvbng_pppoe.OsvbngPppoeAddDelSessionReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("delete pppoe session: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("delete pppoe session failed: retval=%d", reply.Retval)
	}

	v.ifMgr.Remove(uint32(reply.SwIfIndex))

	v.logger.Debug("Deleted PPPoE session from VPP",
		"session_id", sessionID,
		"client_ip", clientIP.String())

	return nil
}


func (v *VPP) AddPPPoESessionAsync(sessionID uint16, clientIP net.IP, clientMAC net.HardwareAddr, localMAC net.HardwareAddr, encapIfIndex uint32, outerVLAN uint16, innerVLAN uint16, decapVrfID uint32, pppMTU uint16, policy southbound.MSSClampPolicy, callback func(uint32, error)) {
	clientAddr, err := v.toAddress(clientIP)
	if err != nil {
		callback(0, fmt.Errorf("convert client IP: %w", err))
		return
	}

	var clientMacAddr ethernet_types.MacAddress
	copy(clientMacAddr[:], clientMAC)

	var localMacAddr ethernet_types.MacAddress
	copy(localMacAddr[:], localMAC)

	req := &osvbng_pppoe.OsvbngPppoeAddDelSession{
		IsAdd:        true,
		SessionID:    sessionID,
		ClientIP:     clientAddr,
		DecapVrfID:   decapVrfID,
		ClientMac:    clientMacAddr,
		LocalMac:     localMacAddr,
		EncapIfIndex: interface_types.InterfaceIndex(encapIfIndex),
		OuterVlan:    outerVLAN,
		InnerVlan:    innerVLAN,
	}

	v.asyncWorker.SendAsync(req, func(reply api.Message, err error) {
		if err != nil {
			callback(0, err)
			return
		}
		r, ok := reply.(*osvbng_pppoe.OsvbngPppoeAddDelSessionReply)
		if !ok {
			callback(0, fmt.Errorf("unexpected reply type: %T", reply))
			return
		}
		if r.Retval != 0 {
			callback(0, fmt.Errorf("VPP error: retval=%d", r.Retval))
			return
		}
		swIdx := uint32(r.SwIfIndex)

		ch, chErr := v.conn.NewAPIChannel()
		if chErr != nil {
			v.rollbackPPPoESession(sessionID, swIdx, clientIP, clientMAC, chErr)
			callback(0, fmt.Errorf("open channel for mtu/clamp on pppoe-session-%d: %w", sessionID, chErr))
			return
		}
		defer ch.Close()

		if err := v.programPPPoESessionMTUAndClamp(ch, sessionID, swIdx, pppMTU, policy, clientIP, clientMAC); err != nil {
			callback(0, err)
			return
		}

		v.ifMgr.Add(&ifmgr.Interface{
			SwIfIndex:    swIdx,
			SupSwIfIndex: encapIfIndex,
			Name:         fmt.Sprintf("pppoe-session-%d", sessionID),
			Type:         ifmgr.IfTypeP2P,
			AdminUp:      true,
			FIBTableID:   decapVrfID,
			MTU:          uint32(pppMTU),
		})
		v.logger.Debug("Added PPPoE session to VPP (async)",
			"session_id", sessionID,
			"client_ip", clientIP.String(),
			"sw_if_index", r.SwIfIndex,
			"ppp_mtu", pppMTU,
			"ipv4_mss", policy.IPv4MSS,
			"ipv6_mss", policy.IPv6MSS)
		callback(swIdx, nil)
	})
}


func (v *VPP) DeletePPPoESessionAsync(sessionID uint16, clientIP net.IP, clientMAC net.HardwareAddr, callback func(error)) {
	clientAddr, err := v.toAddress(clientIP)
	if err != nil {
		callback(fmt.Errorf("convert client IP: %w", err))
		return
	}

	var clientMacAddr ethernet_types.MacAddress
	copy(clientMacAddr[:], clientMAC)

	req := &osvbng_pppoe.OsvbngPppoeAddDelSession{
		IsAdd:     false,
		SessionID: sessionID,
		ClientIP:  clientAddr,
		ClientMac: clientMacAddr,
	}

	v.asyncWorker.SendAsync(req, func(reply api.Message, err error) {
		if err != nil {
			callback(err)
			return
		}
		r, ok := reply.(*osvbng_pppoe.OsvbngPppoeAddDelSessionReply)
		if !ok {
			callback(fmt.Errorf("unexpected reply type: %T", reply))
			return
		}
		if r.Retval != 0 {
			callback(fmt.Errorf("VPP error: retval=%d", r.Retval))
			return
		}
		v.ifMgr.Remove(uint32(r.SwIfIndex))
		v.logger.Debug("Deleted PPPoE session from VPP (async)",
			"session_id", sessionID,
			"client_ip", clientIP.String())
		callback(nil)
	})
}


