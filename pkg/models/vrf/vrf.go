package vrf

import "fmt"

type VRF struct {
	Name            string              `json:"name"`
	Description     string              `json:"description"`
	TableId         uint32              `json:"tableId"`
	AddressFamilies AddressFamilyConfig `json:"addressFamilies"`
}

type AddressFamilyConfig struct {
	IPv4Unicast   *IPv4UnicastAF   `json:"ipv4Unicast,omitempty"`
	IPv6Unicast   *IPv6UnicastAF   `json:"ipv6Unicast,omitempty"`
	IPv4Multicast *IPv4MulticastAF `json:"ipv4Multicast,omitempty"`
	IPv6Multicast *IPv6MulticastAF `json:"ipv6Multicast,omitempty"`
}

type IPv4UnicastAF struct {
	ImportRoutePolicy  string   `json:"importRoutePolicy,omitempty"`
	ExportRoutePolicy  string   `json:"exportRoutePolicy,omitempty"`
	RouteDistinguisher string   `json:"routeDistinguisher,omitempty"`
	ImportRouteTargets []string `json:"importRouteTargets,omitempty"`
	ExportRouteTargets []string `json:"exportRouteTargets,omitempty"`
}

type IPv6UnicastAF struct {
	ImportRoutePolicy  string   `json:"importRoutePolicy,omitempty"`
	ExportRoutePolicy  string   `json:"exportRoutePolicy,omitempty"`
	RouteDistinguisher string   `json:"routeDistinguisher,omitempty"`
	ImportRouteTargets []string `json:"importRouteTargets,omitempty"`
	ExportRouteTargets []string `json:"exportRouteTargets,omitempty"`
}

type IPv4MulticastAF struct {
	ImportRoutePolicy  string   `json:"importRoutePolicy,omitempty"`
	ExportRoutePolicy  string   `json:"exportRoutePolicy,omitempty"`
	RouteDistinguisher string   `json:"routeDistinguisher,omitempty"`
	ImportRouteTargets []string `json:"importRouteTargets,omitempty"`
	ExportRouteTargets []string `json:"exportRouteTargets,omitempty"`
}

type IPv6MulticastAF struct {
	ImportRoutePolicy  string   `json:"importRoutePolicy,omitempty"`
	ExportRoutePolicy  string   `json:"exportRoutePolicy,omitempty"`
	RouteDistinguisher string   `json:"routeDistinguisher,omitempty"`
	ImportRouteTargets []string `json:"importRouteTargets,omitempty"`
	ExportRouteTargets []string `json:"exportRouteTargets,omitempty"`
}

func (v *VRF) HasIPv4() bool {
	return v.AddressFamilies.IPv4Unicast != nil
}

func (v *VRF) HasIPv6() bool {
	return v.AddressFamilies.IPv6Unicast != nil
}

func ValidateVRFName(vrfName string) error {
	if vrfName == "" {
		return fmt.Errorf("VRF name cannot be empty")
	}

	if len(vrfName) > 15 {
		return fmt.Errorf("VRF name too long (max 15 characters): %s", vrfName)
	}

	if !((vrfName[0] >= 'a' && vrfName[0] <= 'z') ||
		(vrfName[0] >= 'A' && vrfName[0] <= 'Z') ||
		(vrfName[0] >= '0' && vrfName[0] <= '9')) {
		return fmt.Errorf("VRF name must start with letter or number: %s", vrfName)
	}

	for _, c := range vrfName {
		if !((c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.') {
			return fmt.Errorf("VRF name contains invalid character '%c': %s", c, vrfName)
		}
	}

	return nil
}
