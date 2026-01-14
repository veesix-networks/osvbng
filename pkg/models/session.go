package models

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

type DHCPv4Session struct {
	SessionID  string
	State      SessionState
	AccessType string
	Protocol   string

	MAC       net.HardwareAddr
	OuterVLAN uint16
	InnerVLAN uint16
	VLANCount int

	InterfaceName string
	IfIndex       int

	IPv4Address net.IP
	ClientID    []byte
	Hostname    string
	DHCPXID     uint32
	DHCPOptions map[uint8][]byte
	RelayInfo   map[uint8][]byte

	RADIUSSessionID  string
	RADIUSAttributes map[string]string

	UpstreamMbps   int
	DownstreamMbps int

	LeaseTime uint32
	CreatedAt time.Time
	LastSeen  time.Time
	ExpiresAt time.Time
}

type DHCPv6Session struct {
	SessionID string
	State     SessionState

	MAC       net.HardwareAddr
	OuterVLAN uint16
	InnerVLAN uint16
	VLANCount int

	InterfaceName string
	IfIndex       int

	IPv6Address net.IP
	IPv6Prefix  string
	DUID        []byte
	DHCPOptions map[uint8][]byte

	RADIUSSessionID  string
	RADIUSAttributes map[string]string

	UpstreamMbps   int
	DownstreamMbps int

	CreatedAt time.Time
	LastSeen  time.Time
	ExpiresAt time.Time
}

type PPPSession struct {
	SessionID    string
	State        SessionState
	PPPSessionID uint16

	MAC       net.HardwareAddr
	OuterVLAN uint16
	InnerVLAN uint16
	VLANCount int

	InterfaceName string
	IfIndex       int

	IPv4Address net.IP
	IPv6Address net.IP
	LCPState    string
	IPCPState   string
	IPv6CPState string

	RADIUSSessionID  string
	RADIUSAttributes map[string]string

	UpstreamMbps   int
	DownstreamMbps int

	CreatedAt time.Time
	LastSeen  time.Time
	ExpiresAt time.Time
}

func MakeDHCPv4SessionID(mac net.HardwareAddr, vlan uint16) string {
	return fmt.Sprintf("ipoe-v4:%s:%d", mac.String(), vlan)
}

func MakeDHCPv6SessionID(mac net.HardwareAddr, vlan uint16) string {
	return fmt.Sprintf("ipoe-v6:%s:%d", mac.String(), vlan)
}

func MakePPPSessionID(sessionID uint16) string {
	return fmt.Sprintf("ppp:%d", sessionID)
}

func (s *DHCPv4Session) RedisKey() string {
	return fmt.Sprintf("osvbng:sessions:%s", s.SessionID)
}

func (s DHCPv4Session) MarshalJSON() ([]byte, error) {
	type Alias DHCPv4Session
	return json.Marshal(&struct {
		MAC         string `json:"MAC"`
		IPv4Address string `json:"IPv4Address"`
		Alias
	}{
		MAC:         s.MAC.String(),
		IPv4Address: s.IPv4Address.String(),
		Alias:       (Alias)(s),
	})
}

func (s *DHCPv4Session) UnmarshalJSON(data []byte) error {
	type Alias DHCPv4Session
	aux := &struct {
		MAC         string `json:"MAC"`
		IPv4Address string `json:"IPv4Address"`
		*Alias
	}{
		Alias: (*Alias)(s),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if aux.MAC != "" {
		s.MAC, _ = net.ParseMAC(aux.MAC)
	}
	if aux.IPv4Address != "" {
		s.IPv4Address = net.ParseIP(aux.IPv4Address)
	}
	return nil
}

func (s *DHCPv6Session) RedisKey() string {
	return fmt.Sprintf("osvbng:sessions:%s", s.SessionID)
}

func (s *PPPSession) RedisKey() string {
	return fmt.Sprintf("osvbng:sessions:%s", s.SessionID)
}

func (s *DHCPv4Session) InterfaceKey() string {
	if s.VLANCount == 0 {
		return "vbng.untagged"
	} else if s.VLANCount == 1 {
		return fmt.Sprintf("vbng.%d", s.OuterVLAN)
	}
	return fmt.Sprintf("vbng.%d.%d", s.OuterVLAN, s.InnerVLAN)
}

func (s *DHCPv6Session) InterfaceKey() string {
	if s.VLANCount == 0 {
		return "vbng.untagged"
	} else if s.VLANCount == 1 {
		return fmt.Sprintf("vbng.%d", s.OuterVLAN)
	}
	return fmt.Sprintf("vbng.%d.%d", s.OuterVLAN, s.InnerVLAN)
}

func (s *PPPSession) InterfaceKey() string {
	if s.VLANCount == 0 {
		return "vbng.untagged"
	} else if s.VLANCount == 1 {
		return fmt.Sprintf("vbng.%d", s.OuterVLAN)
	}
	return fmt.Sprintf("vbng.%d.%d", s.OuterVLAN, s.InnerVLAN)
}

type SessionStats struct {
	RxPackets uint64 `json:"rx_packets"`
	RxBytes   uint64 `json:"rx_bytes"`
	TxPackets uint64 `json:"tx_packets"`
	TxBytes   uint64 `json:"tx_bytes"`
}

func (s *SessionStats) Marshal() ([]byte, error) {
	return json.Marshal(s)
}

func (s *SessionStats) Unmarshal(data []byte) error {
	return json.Unmarshal(data, s)
}
