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
	return p.expand(p.Format, ctx)
}

func (p *AAAPolicy) ExpandPassword(ctx *PolicyContext) string {
	return p.expand(p.Password, ctx)
}

// ExpandFormatChecked expands the policy Format and reports whether the
// result is usable as a User-Name. ok is false when Format is unset (no
// policy username to apply) or when expansion collapses to "" because a
// referenced identity token (e.g. $remote-id$) was absent at request time.
func (p *AAAPolicy) ExpandFormatChecked(ctx *PolicyContext) (string, bool) {
	expanded := p.expand(p.Format, ctx)
	return expanded, p.Format != "" && expanded != ""
}

func (p *AAAPolicy) expand(template string, ctx *PolicyContext) string {
	result := template

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
