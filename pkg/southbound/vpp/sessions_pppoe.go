package vpp

import (
	"fmt"
	"github.com/veesix-networks/osvbng/pkg/ifmgr"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ethernet_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/osvbng_pppoe"
	"go.fd.io/govpp/api"
	"net"
)

func (v *VPP) AddPPPoESession(sessionID uint16, clientIP net.IP, clientMAC net.HardwareAddr, localMAC net.HardwareAddr, encapIfIndex uint32, outerVLAN uint16, innerVLAN uint16, decapVrfID uint32) (uint32, error) {
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
	v.ifMgr.Add(&ifmgr.Interface{
		SwIfIndex:    swIdx,
		SupSwIfIndex: encapIfIndex,
		Name:         fmt.Sprintf("pppoe-session-%d", sessionID),
		Type:         ifmgr.IfTypeP2P,
		AdminUp:      true,
		FIBTableID:   decapVrfID,
	})

	v.logger.Debug("Added PPPoE session to VPP",
		"session_id", sessionID,
		"client_ip", clientIP.String(),
		"client_mac", clientMAC.String(),
		"local_mac", localMAC.String(),
		"encap_if_index", encapIfIndex,
		"outer_vlan", outerVLAN,
		"inner_vlan", innerVLAN,
		"sw_if_index", reply.SwIfIndex)

	return swIdx, nil
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


func (v *VPP) AddPPPoESessionAsync(sessionID uint16, clientIP net.IP, clientMAC net.HardwareAddr, localMAC net.HardwareAddr, encapIfIndex uint32, outerVLAN uint16, innerVLAN uint16, decapVrfID uint32, callback func(uint32, error)) {
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
		v.ifMgr.Add(&ifmgr.Interface{
			SwIfIndex:    swIdx,
			SupSwIfIndex: encapIfIndex,
			Name:         fmt.Sprintf("pppoe-session-%d", sessionID),
			Type:         ifmgr.IfTypeP2P,
			AdminUp:      true,
			FIBTableID:   decapVrfID,
		})
		v.logger.Debug("Added PPPoE session to VPP (async)",
			"session_id", sessionID,
			"client_ip", clientIP.String(),
			"sw_if_index", r.SwIfIndex)
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


