package events

import "github.com/veesix-networks/osvbng/pkg/models"

type SessionLifecycleEvent struct {
	AccessType models.AccessType
	Protocol   models.Protocol
	SessionID  string
	State      models.SessionState
	Session    any
}

type AAARequestEvent struct {
	AccessType models.AccessType
	Protocol   models.Protocol
	SessionID  string
	Request    models.AAARequest
}

type AAAResponseEvent struct {
	AccessType models.AccessType
	SessionID  string
	Response   models.AAAResponse
}

type EgressEvent struct {
	Protocol models.Protocol
	Packet   models.EgressPacketPayload
}
