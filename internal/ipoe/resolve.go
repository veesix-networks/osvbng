// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ipoe

import (
	"net"

	"github.com/veesix-networks/osvbng/pkg/aaa"
	"github.com/veesix-networks/osvbng/pkg/allocator"
	"github.com/veesix-networks/osvbng/pkg/config/ip"
	"github.com/veesix-networks/osvbng/pkg/dhcp"
	"github.com/veesix-networks/osvbng/pkg/dhcp4"
	"github.com/veesix-networks/osvbng/pkg/dhcp6"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/svcgroup"
)

func (c *Component) resolveTargetFromEvent(ev *events.SubscriberMutationEvent) *SessionState {
	if ev.SessionID != "" {
		if val, ok := c.sessions.Load(ev.SessionID); ok {
			return val.(*SessionState)
		}
	}
	if ev.AcctSessionID != "" {
		if val, ok := c.acctSessionIndex.Load(ev.AcctSessionID); ok {
			return val.(*SessionState)
		}
	}
	if ev.Username != "" {
		if val, ok := c.usernameIndex.Load(ev.Username); ok {
			return val.(*SessionState)
		}
	}
	if ev.FramedIPv4 != "" {
		if val, ok := c.ipv4Index.Load(ev.FramedIPv4); ok {
			return val.(*SessionState)
		}
	}
	if ev.FramedIPv6 != "" {
		if val, ok := c.ipv6Index.Load(ev.FramedIPv6); ok {
			return val.(*SessionState)
		}
	}
	return nil
}

func (c *Component) resolveTerminateTarget(ev *events.SubscriberTerminateEvent) *SessionState {
	if ev.Key != nil {
		var mac net.HardwareAddr = ev.Key.MAC[:]
		if val, ok := c.sessions.Load(c.makeSessionKeyV4(mac, ev.Key.SVLAN, ev.Key.CVLAN)); ok {
			return val.(*SessionState)
		}
		if val, ok := c.sessions.Load(c.makeSessionKeyV6(mac, ev.Key.SVLAN, ev.Key.CVLAN)); ok {
			return val.(*SessionState)
		}
	}
	if ev.SessionID != "" {
		if val, ok := c.sessionIndex.Load(ev.SessionID); ok {
			return val.(*SessionState)
		}
	}
	if ev.AcctSessionID != "" {
		if val, ok := c.acctSessionIndex.Load(ev.AcctSessionID); ok {
			return val.(*SessionState)
		}
	}
	if ev.Username != "" {
		if val, ok := c.usernameIndex.Load(ev.Username); ok {
			return val.(*SessionState)
		}
	}
	if ev.FramedIPv4 != "" {
		if val, ok := c.ipv4Index.Load(ev.FramedIPv4); ok {
			return val.(*SessionState)
		}
	}
	if ev.FramedIPv6 != "" {
		if val, ok := c.ipv6Index.Load(ev.FramedIPv6); ok {
			return val.(*SessionState)
		}
	}
	return nil
}

func (c *Component) resolveServiceGroup(svlan, cvlan uint16, aaaAttrs map[string]interface{}) svcgroup.ServiceGroup {
	var sgName string
	if v, ok := aaaAttrs[aaa.AttrServiceGroup]; ok {
		if s, ok := v.(string); ok {
			sgName = s
		}
	}

	var defaultSG string
	if match, ok := c.cfgMgr.LookupSubscriberGroup(svlan, cvlan); ok {
		defaultSG = match.Group.DefaultServiceGroup
	}

	return c.svcGroupResolver.Resolve(sgName, defaultSG, aaaAttrs)
}

func (c *Component) buildAllocContext(sess *SessionState, aaaAttrs map[string]interface{}) *allocator.Context {
	var profileName, ipv6ProfileName string
	if match, ok := c.cfgMgr.LookupSubscriberGroup(sess.OuterVLAN, sess.InnerVLAN); ok {
		profileName = match.Group.IPv4Profile
		ipv6ProfileName = match.Group.IPv6Profile
	}

	ctx := allocator.NewContext(sess.SessionID, sess.MAC, sess.OuterVLAN, sess.InnerVLAN, sess.VRF, sess.ServiceGroup.Name, profileName, ipv6ProfileName, aaaAttrs)

	if ctx.PoolOverride == "" && sess.ServiceGroup.Pool != "" {
		ctx.PoolOverride = sess.ServiceGroup.Pool
	}
	if ctx.IANAPoolOverride == "" && sess.ServiceGroup.IANAPool != "" {
		ctx.IANAPoolOverride = sess.ServiceGroup.IANAPool
	}
	if ctx.PDPoolOverride == "" && sess.ServiceGroup.PDPool != "" {
		ctx.PDPoolOverride = sess.ServiceGroup.PDPool
	}

	if sess.ServiceGroup.IPv4Profile != "" {
		ctx.ProfileName = sess.ServiceGroup.IPv4Profile
	}
	if sess.ServiceGroup.IPv6Profile != "" {
		ctx.IPv6ProfileName = sess.ServiceGroup.IPv6Profile
	}

	return ctx
}

// sessionV4Enabled / sessionV6Enabled answer whether an established session
// serves a family. Once AAA has resolved the session policy, the allocator
// context is authoritative and read-once, so a later config reload does not
// retroactively flip a live session. Before approval there is no resolved
// policy, so the current subscriber-group binding is used.
func (c *Component) sessionV4Enabled(sess *SessionState) bool {
	if sess.AllocCtx != nil {
		return sess.AllocCtx.ProfileName != ""
	}
	if m, ok := c.cfgMgr.LookupSubscriberGroup(sess.OuterVLAN, sess.InnerVLAN); ok {
		return groupV4Enabled(m.Group)
	}
	return false
}

func (c *Component) sessionV6Enabled(sess *SessionState) bool {
	if sess.AllocCtx != nil {
		return sess.AllocCtx.IPv6ProfileName != ""
	}
	if m, ok := c.cfgMgr.LookupSubscriberGroup(sess.OuterVLAN, sess.InnerVLAN); ok {
		return groupV6Enabled(m.Group)
	}
	return false
}

func (c *Component) getDHCP4Provider(profile *ip.IPv4Profile) dhcp4.DHCPProvider {
	mode := "local"
	if profile != nil {
		m := profile.GetMode()
		if m == "relay" || m == "proxy" {
			mode = m
		}
	}
	return c.dhcp4Providers[mode]
}

func (c *Component) getDHCP6Provider(profile *ip.IPv6Profile) dhcp6.DHCPProvider {
	mode := "local"
	if profile != nil {
		m := profile.GetMode()
		if m == "relay" || m == "proxy" {
			mode = m
		}
	}
	return c.dhcp6Providers[mode]
}

func (c *Component) resolveIPv4Profile(ctx *allocator.Context) *ip.IPv4Profile {
	if ctx == nil || ctx.ProfileName == "" {
		return nil
	}
	cfg, err := c.cfgMgr.GetRunning()
	if err != nil || cfg == nil {
		return nil
	}
	return cfg.IPv4Profiles[ctx.ProfileName]
}

func (c *Component) resolveIPv6Profile(ctx *allocator.Context) *ip.IPv6Profile {
	if ctx == nil || ctx.IPv6ProfileName == "" {
		return nil
	}
	cfg, err := c.cfgMgr.GetRunning()
	if err != nil || cfg == nil {
		return nil
	}
	return cfg.IPv6Profiles[ctx.IPv6ProfileName]
}

func (c *Component) accessInterfaceName(encapIfIndex uint32) string {
	if c.ifMgr == nil {
		return ""
	}
	if iface := c.ifMgr.Get(encapIfIndex); iface != nil {
		return iface.Name
	}
	return ""
}

func (c *Component) resolveAccessInterfaceName(encapIfIndex uint32) string {
	iface := c.ifMgr.Get(encapIfIndex)
	if iface == nil {
		return ""
	}
	parent := c.ifMgr.Get(iface.SupSwIfIndex)
	if parent == nil {
		return iface.Name
	}
	return parent.Name
}

func (c *Component) resolveDHCPv4(ctx *allocator.Context) *dhcp.ResolvedDHCPv4 {
	if ctx == nil || ctx.ProfileName == "" {
		return nil
	}
	cfg, err := c.cfgMgr.GetRunning()
	if err != nil || cfg == nil {
		return nil
	}
	profile := cfg.IPv4Profiles[ctx.ProfileName]
	if profile == nil {
		c.logger.Error("IPv4 profile not found", "profile", ctx.ProfileName, "session_id", ctx.SessionID)
		return nil
	}
	resolved := dhcp.ResolveV4(ctx, profile)
	if resolved == nil {
		c.logger.Warn("IPv4 address resolution failed",
			"session_id", ctx.SessionID,
			"profile", ctx.ProfileName,
			"vrf", ctx.VRF,
			"pool_override", ctx.PoolOverride)
	}
	return resolved
}

func (c *Component) resolveDHCPv6(ctx *allocator.Context) *dhcp.ResolvedDHCPv6 {
	if ctx == nil || ctx.IPv6ProfileName == "" {
		return nil
	}
	cfg, err := c.cfgMgr.GetRunning()
	if err != nil || cfg == nil {
		return nil
	}
	profile := cfg.IPv6Profiles[ctx.IPv6ProfileName]
	if profile == nil {
		c.logger.Error("IPv6 profile not found", "profile", ctx.IPv6ProfileName, "session_id", ctx.SessionID)
		return nil
	}
	return dhcp.ResolveV6(ctx, profile)
}

func (c *Component) resolveSRGName(svlan, cvlan uint16) string {
	if c.srgMgr == nil {
		return ""
	}
	match, ok := c.cfgMgr.LookupSubscriberGroup(svlan, cvlan)
	if !ok {
		return ""
	}
	return c.srgMgr.GetSRGForGroup(match.Name)
}

func (c *Component) allowRelayForward(svlan, cvlan uint16) bool {
	match, ok := c.cfgMgr.LookupSubscriberGroup(svlan, cvlan)
	if !ok {
		return true
	}
	return match.Group.GetAllowRelayForward()
}

func (c *Component) resolveUnnumberedLoopback(sess *SessionState) string {
	if sess.ServiceGroup.Unnumbered != "" {
		return sess.ServiceGroup.Unnumbered
	}

	if match, ok := c.cfgMgr.LookupSubscriberGroup(sess.OuterVLAN, sess.InnerVLAN); ok && match.VR != nil {
		return match.VR.Interface
	}

	return ""
}

func (c *Component) setupSessionUnnumbered(sessID string, swIfIndex uint32, loopback string) {
	if loopback == "" {
		return
	}

	c.vpp.SetUnnumberedAsync(swIfIndex, loopback, func(err error) {
		if err != nil {
			c.logger.Error("Failed to set unnumbered on IPoE session", "session_id", sessID, "sw_if_index", swIfIndex, "loopback", loopback, "error", err)
		}
	})
}
