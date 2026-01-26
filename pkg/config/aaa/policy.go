package aaa

import (
	"fmt"
	"net"
	"strings"
)

type PolicyContext struct {
	MACAddress     net.HardwareAddr
	SVLAN          uint16
	CVLAN          uint16
	RemoteID       string
	CircuitID      string
	AgentCircuitID string
	AgentRemoteID  string
	AgentRelayID   string
	Hostname       string
}

func (p *AAAPolicy) ExpandFormat(ctx *PolicyContext) string {
	result := p.Format

	remoteID := ctx.RemoteID
	if ctx.AgentRemoteID != "" {
		remoteID = ctx.AgentRemoteID
	}
	circuitID := ctx.CircuitID
	if ctx.AgentCircuitID != "" {
		circuitID = ctx.AgentCircuitID
	}

	result = strings.ReplaceAll(result, "$mac-address$", ctx.MACAddress.String())
	result = strings.ReplaceAll(result, "$svlan$", fmt.Sprintf("%d", ctx.SVLAN))
	result = strings.ReplaceAll(result, "$cvlan$", fmt.Sprintf("%d", ctx.CVLAN))
	result = strings.ReplaceAll(result, "$remote-id$", remoteID)
	result = strings.ReplaceAll(result, "$circuit-id$", circuitID)
	result = strings.ReplaceAll(result, "$agent-circuit-id$", ctx.AgentCircuitID)
	result = strings.ReplaceAll(result, "$agent-remote-id$", ctx.AgentRemoteID)
	result = strings.ReplaceAll(result, "$agent-relay-id$", ctx.AgentRelayID)
	result = strings.ReplaceAll(result, "$hostname$", ctx.Hostname)

	return result
}

func (p *AAAPolicy) ExpandFormatWithLog(ctx *PolicyContext, logger interface{ Debug(string, ...any) }) string {
	result := p.ExpandFormat(ctx)

	logger.Debug("Policy expanded",
		"format", p.Format,
		"result", result,
		"mac", ctx.MACAddress.String(),
		"svlan", ctx.SVLAN,
		"cvlan", ctx.CVLAN,
		"agent_circuit_id", ctx.AgentCircuitID,
		"agent_remote_id", ctx.AgentRemoteID,
		"circuit_id", ctx.CircuitID,
		"remote_id", ctx.RemoteID)

	return result
}
