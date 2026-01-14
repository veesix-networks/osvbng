package models

import (
	"encoding/json"
	"time"
)

type Event struct {
	ID         string     `json:"event_id"`
	Type       EventType  `json:"event_type"`
	Timestamp  time.Time  `json:"timestamp"`
	AccessType AccessType `json:"access_type"`
	Protocol   Protocol   `json:"protocol"`
	SessionID  string     `json:"session_id,omitempty"`

	Payload json.RawMessage `json:"payload,omitempty"`
}

func (e *Event) GetPayload(v interface{}) error {
	if len(e.Payload) == 0 {
		return nil
	}
	return json.Unmarshal(e.Payload, v)
}

func (e *Event) SetPayload(v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	e.Payload = data
	return nil
}

type AAARequest struct {
	RequestID     string `json:"request_id"`
	Username      string `json:"username"`
	MAC           string `json:"mac"`
	NASIPAddress  string `json:"nas_ip"`
	NASPort       uint32 `json:"nas_port"`
	AcctSessionID string `json:"acct_session_id"`
}

type AAAResponse struct {
	RequestID  string                 `json:"request_id"`
	Allowed    bool                   `json:"allowed"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
	Error      string                 `json:"error,omitempty"`
}

type DHCPRelayPayload struct {
	RawData   []byte `json:"raw_data"`
	MAC       string `json:"mac"`
	OuterVLAN uint16 `json:"outer_vlan"`
	InnerVLAN uint16 `json:"inner_vlan"`
}

type DHCPResponsePayload struct {
	RawData   []byte `json:"raw_data"`
	MAC       string `json:"mac"`
	OuterVLAN uint16 `json:"outer_vlan"`
	InnerVLAN uint16 `json:"inner_vlan"`
	XID       uint32 `json:"xid"`
}

type ARPPacketPayload struct {
	RawData   []byte `json:"raw_data"`
	TargetIP  string `json:"target_ip"`
	OuterVLAN uint16 `json:"outer_vlan"`
	InnerVLAN uint16 `json:"inner_vlan"`
}

type EgressPacketPayload struct {
	RawData   []byte `json:"raw_data"`
	DstMAC    string `json:"dst_mac"`
	SrcMAC    string `json:"src_mac"`
	OuterVLAN uint16 `json:"outer_vlan"`
	InnerVLAN uint16 `json:"inner_vlan"`
}

type EventType string

const (
	EventTypePacket           EventType = "packet"
	EventTypeSessionLifecycle EventType = "session_lifecycle"
	EventTypeAAARequest       EventType = "aaa_request"
	EventTypeAAAResponse      EventType = "aaa_response"
	EventTypeDHCPRelay        EventType = "dhcp_relay"
	EventTypeDHCPResponse     EventType = "dhcp_response"
	EventTypeARPRequest       EventType = "arp_request"
	EventTypeSubscriberState  EventType = "subscriber_state"
	EventTypeEgress           EventType = "egress"
)

type AccessType string

const (
	AccessTypeIPoE    AccessType = "ipoe"
	AccessTypePPPoE   AccessType = "pppoe"
	AccessTypeL2TP    AccessType = "l2tp"
	AccessTypeUnknown AccessType = "unknown"
)

type Protocol string

const (
	ProtocolDHCPv4  Protocol = "dhcpv4"
	ProtocolDHCPv6  Protocol = "dhcpv6"
	ProtocolARP     Protocol = "arp"
	ProtocolPPP     Protocol = "ppp"
	ProtocolL2TP    Protocol = "l2tp"
	ProtocolUnknown Protocol = "unknown"
)

type SessionState string

const (
	SessionStateUnknown     SessionState = "unknown"
	SessionStateDiscovering SessionState = "discovering"
	SessionStateOffered     SessionState = "offered"
	SessionStateRequesting  SessionState = "requesting"
	SessionStateActive      SessionState = "active"
	SessionStateReleased    SessionState = "released"
)
