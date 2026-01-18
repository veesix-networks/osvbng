package config

import (
	"fmt"
	"net"
	"strings"
)

type PolicyContext struct {
	MACAddress    net.HardwareAddr
	SVLAN         uint16
	CVLAN         uint16
	NASPort       uint32
	RemoteID      string
	CircuitID     string
	AgentRelayID  string
	Hostname      string
}

func (p *AAAPolicy) ExpandFormat(ctx *PolicyContext) string {
	result := p.Format

	result = strings.ReplaceAll(result, "$mac-address$", ctx.MACAddress.String())
	result = strings.ReplaceAll(result, "$svlan$", fmt.Sprintf("%d", ctx.SVLAN))
	result = strings.ReplaceAll(result, "$cvlan$", fmt.Sprintf("%d", ctx.CVLAN))
	result = strings.ReplaceAll(result, "$nas-port$", fmt.Sprintf("%d", ctx.NASPort))
	result = strings.ReplaceAll(result, "$remote-id$", ctx.RemoteID)
	result = strings.ReplaceAll(result, "$circuit-id$", ctx.CircuitID)
	result = strings.ReplaceAll(result, "$agent-relay-id$", ctx.AgentRelayID)
	result = strings.ReplaceAll(result, "$hostname$", ctx.Hostname)

	return result
}
