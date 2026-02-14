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
	GetIPv6Prefix() string
	GetIfIndex() uint32
	GetVLANCount() int
	GetInterfaceName() string
	GetUsername() string
}

type IPoESession struct {
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
	VRF           string
	ServiceGroup  string

	IPv4Address net.IP
	LeaseTime   uint32
	ClientID    []byte
	Hostname    string
	DHCPXID     uint32
	DHCPOptions map[uint8][]byte
	RelayInfo   map[uint8][]byte

	IPv6Address   net.IP
	IPv6Prefix    string
	IPv6LeaseTime uint32
	DUID          []byte

	Username string

	RADIUSSessionID  string
	RADIUSAttributes map[string]string

	UpstreamMbps   int
	DownstreamMbps int

	CreatedAt time.Time
	LastSeen  time.Time
	ExpiresAt time.Time
}

func (s *IPoESession) GetSessionID() string       { return s.SessionID }
func (s *IPoESession) GetAccessType() AccessType  { return AccessTypeIPoE }
func (s *IPoESession) GetProtocol() Protocol      { return Protocol(s.Protocol) }
func (s *IPoESession) GetState() SessionState     { return s.State }
func (s *IPoESession) GetMAC() net.HardwareAddr   { return s.MAC }
func (s *IPoESession) GetOuterVLAN() uint16       { return s.OuterVLAN }
func (s *IPoESession) GetInnerVLAN() uint16       { return s.InnerVLAN }
func (s *IPoESession) GetRADIUSSessionID() string { return s.RADIUSSessionID }
func (s *IPoESession) GetIPv4Address() net.IP     { return s.IPv4Address }
func (s *IPoESession) GetIPv6Address() net.IP     { return s.IPv6Address }
func (s *IPoESession) GetIPv6Prefix() string      { return s.IPv6Prefix }
func (s *IPoESession) GetIfIndex() uint32         { return s.IfIndex }
func (s *IPoESession) GetVLANCount() int          { return s.VLANCount }
func (s *IPoESession) GetInterfaceName() string   { return s.InterfaceName }
func (s *IPoESession) GetUsername() string         { return s.Username }

func (s *IPoESession) IsDualStack() bool {
	return s.IPv4Address != nil && (s.IPv6Address != nil || s.IPv6Prefix != "")
}

func (s *IPoESession) RedisKey() string {
	return fmt.Sprintf("osvbng:sessions:%s", s.SessionID)
}

func (s IPoESession) MarshalJSON() ([]byte, error) {
	type Alias IPoESession
	aux := &struct {
		MAC         string `json:"MAC,omitempty"`
		IPv4Address string `json:"IPv4Address,omitempty"`
		IPv6Address string `json:"IPv6Address,omitempty"`
		Alias
	}{
		Alias: (Alias)(s),
	}
	if s.MAC != nil {
		aux.MAC = s.MAC.String()
	}
	if s.IPv4Address != nil {
		aux.IPv4Address = s.IPv4Address.String()
	}
	if s.IPv6Address != nil {
		aux.IPv6Address = s.IPv6Address.String()
	}
	return json.Marshal(aux)
}

func (s *IPoESession) UnmarshalJSON(data []byte) error {
	type Alias IPoESession
	aux := &struct {
		MAC         string `json:"MAC"`
		IPv4Address string `json:"IPv4Address"`
		IPv6Address string `json:"IPv6Address"`
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
	if aux.IPv4Address != "" && aux.IPv4Address != "<nil>" {
		s.IPv4Address = net.ParseIP(aux.IPv4Address)
	}
	if aux.IPv6Address != "" && aux.IPv6Address != "<nil>" {
		s.IPv6Address = net.ParseIP(aux.IPv6Address)
	}
	return nil
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
	VRF           string
	ServiceGroup  string

	IPv4Address net.IP
	IPv6Address net.IP
	IPv6Prefix  string
	LCPState    string
	IPCPState   string
	IPv6CPState string

	Username string

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
func (s *PPPSession) GetIPv6Prefix() string      { return s.IPv6Prefix }
func (s *PPPSession) GetIfIndex() uint32         { return s.IfIndex }
func (s *PPPSession) GetVLANCount() int          { return s.VLANCount }
func (s *PPPSession) GetInterfaceName() string   { return s.InterfaceName }
func (s *PPPSession) GetUsername() string         { return s.Username }

func (s *PPPSession) RedisKey() string {
	return fmt.Sprintf("osvbng:sessions:%s", s.SessionID)
}

func MakeIPoESessionID(mac net.HardwareAddr, vlan uint16) string {
	return fmt.Sprintf("ipoe:%s:%d", mac.String(), vlan)
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
