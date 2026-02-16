package configmgr

import (
	"fmt"
	"net"

	"github.com/veesix-networks/osvbng/pkg/operations"
	"github.com/veesix-networks/osvbng/pkg/southbound/vpp"
)

var dhcpv6MulticastAddr = net.ParseIP("ff02::1:2")

type DataplaneState struct {
	Interfaces             map[uint32]*operations.InterfaceState
	InterfacesByName       map[string]*operations.InterfaceState
	Unnumbered             map[uint32]uint32
	IPv6Enabled            map[uint32]bool
	IPv6RA                 map[uint32]*operations.IPv6RAState
	PuntRegistrations      map[uint32]map[uint8]bool
	DHCPv6MulticastEnabled map[uint32]bool
}

func NewDataplaneState() *DataplaneState {
	return &DataplaneState{
		Interfaces:             make(map[uint32]*operations.InterfaceState),
		InterfacesByName:       make(map[string]*operations.InterfaceState),
		Unnumbered:             make(map[uint32]uint32),
		IPv6Enabled:            make(map[uint32]bool),
		IPv6RA:                 make(map[uint32]*operations.IPv6RAState),
		PuntRegistrations:      make(map[uint32]map[uint8]bool),
		DHCPv6MulticastEnabled: make(map[uint32]bool),
	}
}

func (ds *DataplaneState) LoadFromDataplane(vpp *vpp.VPP) error {
	interfaces, err := vpp.DumpInterfaces()
	if err != nil {
		return fmt.Errorf("dump interfaces: %w", err)
	}

	for _, iface := range interfaces {
		state := &operations.InterfaceState{
			SwIfIndex:     iface.SwIfIndex,
			Name:          iface.Name,
			AdminUp:       iface.AdminUp,
			LinkUp:        iface.LinkUp,
			MTU:           iface.MTU,
			IPv4Addresses: []string{},
			IPv6Addresses: []string{},
			OuterVlanID:   iface.OuterVlanID,
			InnerVlanID:   iface.InnerVlanID,
			SupSwIfIndex:  iface.SupSwIfIndex,
		}
		ds.Interfaces[iface.SwIfIndex] = state
		ds.InterfacesByName[iface.Name] = state
	}

	// Load IP addresses
	addresses, err := vpp.DumpIPAddresses()
	if err != nil {
		return fmt.Errorf("dump ip addresses: %w", err)
	}

	for _, addr := range addresses {
		if state, exists := ds.Interfaces[addr.SwIfIndex]; exists {
			if addr.IsIPv6 {
				state.IPv6Addresses = append(state.IPv6Addresses, addr.Address)
			} else {
				state.IPv4Addresses = append(state.IPv4Addresses, addr.Address)
			}
		}
	}

	unnumbered, err := vpp.DumpUnnumbered()
	if err != nil {
		return fmt.Errorf("dump unnumbered: %w", err)
	}

	for _, u := range unnumbered {
		ds.Unnumbered[u.SwIfIndex] = u.IPSwIfIndex
	}

	ipv6Enabled, err := vpp.DumpIPv6Enabled()
	if err != nil {
		return fmt.Errorf("dump ipv6 enabled: %w", err)
	}

	for _, swIfIndex := range ipv6Enabled {
		ds.IPv6Enabled[swIfIndex] = true
	}

	raConfigs, err := vpp.DumpIPv6RA()
	if err != nil {
		return fmt.Errorf("dump ipv6 ra: %w", err)
	}

	for _, ra := range raConfigs {
		ds.IPv6RA[ra.SwIfIndex] = &operations.IPv6RAState{
			SwIfIndex:          ra.SwIfIndex,
			Managed:            ra.Managed,
			Other:              ra.Other,
			RouterLifetimeSecs: ra.RouterLifetimeSecs,
			MaxIntervalSecs:    ra.MaxIntervalSecs,
			MinIntervalSecs:    ra.MinIntervalSecs,
			SendRadv:           ra.SendRadv,
		}
	}

	punts, err := vpp.DumpPuntRegistrations()
	if err != nil {
		return fmt.Errorf("dump punt registrations: %w", err)
	}

	for _, p := range punts {
		if ds.PuntRegistrations[p.SwIfIndex] == nil {
			ds.PuntRegistrations[p.SwIfIndex] = make(map[uint8]bool)
		}
		ds.PuntRegistrations[p.SwIfIndex][p.Protocol] = true
	}

	mroutes, err := vpp.DumpMroutes()
	if err != nil {
		return fmt.Errorf("dump mroutes: %w", err)
	}

	for _, m := range mroutes {
		if m.IsIPv6 && m.GrpAddress.Equal(dhcpv6MulticastAddr) {
			ds.DHCPv6MulticastEnabled[0] = true
			break
		}
	}

	return nil
}

func (ds *DataplaneState) IsInterfaceConfigured(name string) bool {
	_, exists := ds.InterfacesByName[name]
	return exists
}

func (ds *DataplaneState) GetInterfaceByName(name string) *operations.InterfaceState {
	return ds.InterfacesByName[name]
}

func (ds *DataplaneState) IsInterfaceEnabled(name string) bool {
	state := ds.InterfacesByName[name]
	if state == nil {
		return false
	}
	return state.AdminUp
}

func (ds *DataplaneState) GetInterfaceMTU(name string) uint32 {
	state := ds.InterfacesByName[name]
	if state == nil {
		return 0
	}
	return state.MTU
}

func (ds *DataplaneState) HasIPv4Address(name string, addr string) bool {
	state := ds.InterfacesByName[name]
	if state == nil {
		return false
	}
	for _, a := range state.IPv4Addresses {
		if a == addr {
			return true
		}
	}
	return false
}

func (ds *DataplaneState) HasIPv6Address(name string, addr string) bool {
	state := ds.InterfacesByName[name]
	if state == nil {
		return false
	}
	for _, a := range state.IPv6Addresses {
		if a == addr {
			return true
		}
	}
	return false
}

func (ds *DataplaneState) IsUnnumberedConfigured(swIfIndex uint32) bool {
	_, exists := ds.Unnumbered[swIfIndex]
	return exists
}

func (ds *DataplaneState) GetUnnumberedTarget(swIfIndex uint32) (uint32, bool) {
	target, exists := ds.Unnumbered[swIfIndex]
	return target, exists
}

func (ds *DataplaneState) IsIPv6Enabled(swIfIndex uint32) bool {
	return ds.IPv6Enabled[swIfIndex]
}

func (ds *DataplaneState) IsPuntEnabled(swIfIndex uint32, protocol uint8) bool {
	if protos, exists := ds.PuntRegistrations[swIfIndex]; exists {
		return protos[protocol]
	}
	return false
}

func (ds *DataplaneState) GetIPv6RAConfig(swIfIndex uint32) *operations.IPv6RAState {
	return ds.IPv6RA[swIfIndex]
}

func (ds *DataplaneState) IsDHCPv6MulticastEnabled(swIfIndex uint32) bool {
	return ds.DHCPv6MulticastEnabled[swIfIndex]
}
