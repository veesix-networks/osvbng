package models

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

type SubscriberSession interface {
	GetSessionID() string
	GetAccessType() AccessType
	GetProtocol() Protocol
	GetState() SessionState
	GetMAC() net.HardwareAddr
	GetOuterVLAN() uint16
	GetInnerVLAN() uint16
	GetRADIUSSessionID() string
	GetIPv4Address() net.IP
	GetIPv6Address() net.IP
	GetIfIndex() uint32
	GetVLANCount() int
	GetInterfaceName() string
}

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
	IfIndex       uint32

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

func (s *DHCPv4Session) GetSessionID() string       { return s.SessionID }
func (s *DHCPv4Session) GetAccessType() AccessType  { return AccessTypeIPoE }
func (s *DHCPv4Session) GetProtocol() Protocol      { return ProtocolDHCPv4 }
func (s *DHCPv4Session) GetState() SessionState     { return s.State }
func (s *DHCPv4Session) GetMAC() net.HardwareAddr   { return s.MAC }
func (s *DHCPv4Session) GetOuterVLAN() uint16       { return s.OuterVLAN }
func (s *DHCPv4Session) GetInnerVLAN() uint16       { return s.InnerVLAN }
func (s *DHCPv4Session) GetRADIUSSessionID() string { return s.RADIUSSessionID }
func (s *DHCPv4Session) GetIPv4Address() net.IP     { return s.IPv4Address }
func (s *DHCPv4Session) GetIPv6Address() net.IP     { return nil }
func (s *DHCPv4Session) GetIfIndex() uint32         { return s.IfIndex }
func (s *DHCPv4Session) GetVLANCount() int          { return s.VLANCount }
func (s *DHCPv4Session) GetInterfaceName() string   { return s.InterfaceName }

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

type DHCPv6Session struct {
	SessionID  string
	State      SessionState
	AccessType string
	Protocol   string

	MAC       net.HardwareAddr
	OuterVLAN uint16
	InnerVLAN uint16
	VLANCount int

	InterfaceName string
	IfIndex       uint32

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

func (s *DHCPv6Session) GetSessionID() string       { return s.SessionID }
func (s *DHCPv6Session) GetAccessType() AccessType  { return AccessTypeIPoE }
func (s *DHCPv6Session) GetProtocol() Protocol      { return ProtocolDHCPv6 }
func (s *DHCPv6Session) GetState() SessionState     { return s.State }
func (s *DHCPv6Session) GetMAC() net.HardwareAddr   { return s.MAC }
func (s *DHCPv6Session) GetOuterVLAN() uint16       { return s.OuterVLAN }
func (s *DHCPv6Session) GetInnerVLAN() uint16       { return s.InnerVLAN }
func (s *DHCPv6Session) GetRADIUSSessionID() string { return s.RADIUSSessionID }
func (s *DHCPv6Session) GetIPv4Address() net.IP     { return nil }
func (s *DHCPv6Session) GetIPv6Address() net.IP     { return s.IPv6Address }
func (s *DHCPv6Session) GetIfIndex() uint32         { return s.IfIndex }
func (s *DHCPv6Session) GetVLANCount() int          { return s.VLANCount }
func (s *DHCPv6Session) GetInterfaceName() string   { return s.InterfaceName }

func (s *DHCPv6Session) RedisKey() string {
	return fmt.Sprintf("osvbng:sessions:%s", s.SessionID)
}

type PPPSession struct {
	SessionID    string
	State        SessionState
	AccessType   string
	Protocol     string
	PPPSessionID uint16

	MAC       net.HardwareAddr
	OuterVLAN uint16
	InnerVLAN uint16
	VLANCount int

	InterfaceName string
	IfIndex       uint32

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

func (s *PPPSession) GetSessionID() string       { return s.SessionID }
func (s *PPPSession) GetAccessType() AccessType  { return AccessTypePPPoE }
func (s *PPPSession) GetProtocol() Protocol      { return ProtocolPPPoESession }
func (s *PPPSession) GetState() SessionState     { return s.State }
func (s *PPPSession) GetMAC() net.HardwareAddr   { return s.MAC }
func (s *PPPSession) GetOuterVLAN() uint16       { return s.OuterVLAN }
func (s *PPPSession) GetInnerVLAN() uint16       { return s.InnerVLAN }
func (s *PPPSession) GetRADIUSSessionID() string { return s.RADIUSSessionID }
func (s *PPPSession) GetIPv4Address() net.IP     { return s.IPv4Address }
func (s *PPPSession) GetIPv6Address() net.IP     { return s.IPv6Address }
func (s *PPPSession) GetIfIndex() uint32         { return s.IfIndex }
func (s *PPPSession) GetVLANCount() int          { return s.VLANCount }
func (s *PPPSession) GetInterfaceName() string   { return s.InterfaceName }

func (s *PPPSession) RedisKey() string {
	return fmt.Sprintf("osvbng:sessions:%s", s.SessionID)
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
