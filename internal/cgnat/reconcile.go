// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package cgnat

import (
	"context"
	"fmt"
	"hash/fnv"
	"net"

	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/config/cgnat"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

type vrfResolver interface {
	ResolveVRF(name string) (uint32, bool, bool, error)
}

type reconcileDeps struct {
	dp        southbound.CGNATDataplane
	vrf       vrfResolver
	pools     *PoolManager
	blacklist *BlacklistManager
	poolIDMap map[string]uint32
	log       *logger.Logger
}

func poolID(name string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	id := h.Sum32()
	if id == 0 {
		id = 1
	}
	return id
}

type poolAction int

const (
	poolActionNoop poolAction = iota
	poolActionAdd
	poolActionSoftUpdate
	poolActionReplace
	poolActionDrop
)

type poolPlan struct {
	name           string
	poolID         uint32
	action         poolAction
	driftFields    []string
	desired        *cgnat.Pool
	currentInVPP   *southbound.CGNATPoolState
	activeMappings uint32
}

type insidePrefixKey struct {
	poolID uint32
	prefix string
	vrfID  uint32
}

type outsideAddressKey struct {
	poolID uint32
	prefix string
}

func (c *Component) reconcile(ctx context.Context, cfg *config.Config) error {
	deps := reconcileDeps{
		dp:        c.dataplane,
		vrf:       c.vrfMgr,
		pools:     c.pools,
		blacklist: c.blacklist,
		poolIDMap: c.poolIDMap,
		log:       c.logger,
	}
	return reconcileWith(ctx, deps, cfg)
}

func reconcileWith(ctx context.Context, deps reconcileDeps, cfg *config.Config) error {
	if cfg.CGNAT == nil {
		return nil
	}

	desiredPools := cfg.CGNAT.Pools
	rc := cfg.CGNAT.Reconcile

	actualPools, err := deps.dp.CGNATPoolDump()
	if err != nil {
		return fmt.Errorf("cgnat: reconcile: dump pools: %w", err)
	}
	actualPoolByID := make(map[uint32]*southbound.CGNATPoolState, len(actualPools))
	for i := range actualPools {
		actualPoolByID[actualPools[i].PoolID] = &actualPools[i]
	}

	actualInside, err := deps.dp.CGNATPoolInsidePrefixDump(0)
	if err != nil {
		return fmt.Errorf("cgnat: reconcile: dump inside prefixes: %w", err)
	}
	actualOutside, err := deps.dp.CGNATPoolOutsideAddressDump(0)
	if err != nil {
		return fmt.Errorf("cgnat: reconcile: dump outside addresses: %w", err)
	}

	plans := make([]poolPlan, 0, len(desiredPools)+len(actualPools))
	seenID := make(map[uint32]string, len(desiredPools))
	desiredIDByName := make(map[string]uint32, len(desiredPools))
	for name, p := range desiredPools {
		if p == nil {
			continue
		}
		id := poolID(name)
		if other, dup := seenID[id]; dup {
			return fmt.Errorf("cgnat: reconcile: deterministic pool ID collision between %q and %q (id=%d); rename one pool", name, other, id)
		}
		seenID[id] = name
		desiredIDByName[name] = id

		plan := poolPlan{name: name, poolID: id, desired: p}
		if cur, ok := actualPoolByID[id]; ok {
			plan.currentInVPP = cur
			plan.activeMappings = cur.ActiveMappings
			hard, fields := poolHardDrift(cur, p)
			if hard {
				plan.action = poolActionReplace
				plan.driftFields = fields
			} else if poolSoftDrift(cur, p) {
				plan.action = poolActionSoftUpdate
			} else {
				plan.action = poolActionNoop
			}
		} else {
			plan.action = poolActionAdd
		}
		plans = append(plans, plan)
	}
	for id, cur := range actualPoolByID {
		if _, wanted := seenID[id]; wanted {
			continue
		}
		plans = append(plans, poolPlan{
			poolID:         id,
			action:         poolActionDrop,
			currentInVPP:   cur,
			activeMappings: cur.ActiveMappings,
		})
	}

	if err := preflight(plans, desiredPools, desiredIDByName, actualInside, actualOutside, deps.vrf, rc); err != nil {
		return err
	}

	for i := range plans {
		plan := &plans[i]
		switch plan.action {
		case poolActionNoop:
			continue
		case poolActionAdd:
			if err := applyPoolAdd(deps.dp, plan); err != nil {
				return fmt.Errorf("cgnat: reconcile: add pool %q: %w", plan.name, err)
			}
			if deps.log != nil {
				deps.log.Info("cgnat reconcile: add pool", "pool", plan.name, "pool_id", plan.poolID)
			}
		case poolActionSoftUpdate:
			if err := applyPoolAdd(deps.dp, plan); err != nil {
				return fmt.Errorf("cgnat: reconcile: soft-update pool %q: %w", plan.name, err)
			}
			if deps.log != nil {
				deps.log.Info("cgnat reconcile: soft-update pool", "pool", plan.name, "pool_id", plan.poolID)
			}
		case poolActionReplace:
			if err := applyPoolAdd(deps.dp, plan); err != nil {
				return fmt.Errorf("cgnat: reconcile: replace pool %q: %w", plan.name, err)
			}
			if deps.log != nil {
				deps.log.Warn("cgnat reconcile: replace pool",
					"pool", plan.name, "pool_id", plan.poolID,
					"drift_fields", plan.driftFields,
					"dropped_mappings", plan.activeMappings)
			}
		case poolActionDrop:
			if !rc.GetDropOrphans() {
				if deps.log != nil {
					deps.log.Warn("cgnat reconcile: orphan pool kept (drop_orphans=false)",
						"pool_id", plan.poolID, "active_mappings", plan.activeMappings)
				}
				continue
			}
			if err := deps.dp.CGNATPoolAddDel(plan.poolID, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, [4]uint32{}, false); err != nil {
				return fmt.Errorf("cgnat: reconcile: drop orphan pool id=%d: %w", plan.poolID, err)
			}
			if deps.log != nil {
				deps.log.Warn("cgnat reconcile: drop orphan pool",
					"pool_id", plan.poolID, "dropped_mappings", plan.activeMappings)
			}
		}
	}

	if err := reconcileChildren(deps, plans, desiredPools, desiredIDByName, actualInside, actualOutside, rc); err != nil {
		return err
	}

	if err := verify(deps.dp, plans, desiredPools, desiredIDByName); err != nil {
		return fmt.Errorf("cgnat: reconcile: post-apply verify: %w", err)
	}

	populateLocalState(deps, desiredPools, desiredIDByName)

	if rc.GetOnDivergence() == "fail" {
		for _, p := range plans {
			if p.action != poolActionNoop {
				return fmt.Errorf("cgnat: reconcile: on_divergence=fail; converged state but pool %q action=%d ran", p.name, p.action)
			}
		}
	}
	return nil
}

func poolHardDrift(cur *southbound.CGNATPoolState, p *cgnat.Pool) (bool, []string) {
	var drift []string
	mode := uint8(0)
	if p.GetMode() == "deterministic" {
		mode = 1
	}
	if cur.Mode != mode {
		drift = append(drift, "mode")
	}
	addrPool := uint8(0)
	if p.GetAddressPooling() == "arbitrary" {
		addrPool = 1
	}
	if cur.AddressPooling != addrPool {
		drift = append(drift, "address_pooling")
	}
	filt := uint8(0)
	if p.GetFiltering() == "endpoint-dependent" {
		filt = 1
	}
	if cur.Filtering != filt {
		drift = append(drift, "filtering")
	}
	if cur.BlockSize != p.GetBlockSize() {
		drift = append(drift, "block_size")
	}
	if cur.PortRangeStart != p.GetPortRangeStart() || cur.PortRangeEnd != p.GetPortRangeEnd() {
		drift = append(drift, "port_range")
	}
	return len(drift) > 0, drift
}

func poolSoftDrift(cur *southbound.CGNATPoolState, p *cgnat.Pool) bool {
	if cur.MaxBlocksPerSub != p.GetMaxBlocksPerSubscriber() {
		return true
	}
	if cur.MaxSessionsPerSub != p.GetMaxSessionsPerSubscriber() {
		return true
	}
	if cur.PortReuseTimeout != p.GetPortReuseTimeout() {
		return true
	}
	if cur.ALGBitmask != p.GetALGBitmask() {
		return true
	}
	t := p.GetTimeouts()
	if cur.Timeouts[0] != t.TCPEstablished || cur.Timeouts[1] != t.TCPTransitory ||
		cur.Timeouts[2] != t.UDP || cur.Timeouts[3] != t.ICMP {
		return true
	}
	return false
}

func applyPoolAdd(dp southbound.CGNATDataplane, plan *poolPlan) error {
	p := plan.desired
	mode := uint8(0)
	if p.GetMode() == "deterministic" {
		mode = 1
	}
	addrPool := uint8(0)
	if p.GetAddressPooling() == "arbitrary" {
		addrPool = 1
	}
	filt := uint8(0)
	if p.GetFiltering() == "endpoint-dependent" {
		filt = 1
	}
	t := p.GetTimeouts()
	return dp.CGNATPoolAddDel(plan.poolID, mode, addrPool, filt,
		p.GetBlockSize(), p.GetMaxBlocksPerSubscriber(),
		p.GetMaxSessionsPerSubscriber(), p.GetPortRangeStart(),
		p.GetPortRangeEnd(), p.GetPortReuseTimeout(),
		p.GetALGBitmask(),
		[4]uint32{t.TCPEstablished, t.TCPTransitory, t.UDP, t.ICMP}, true)
}

func preflight(plans []poolPlan, desiredPools map[string]*cgnat.Pool,
	desiredIDByName map[string]uint32, actualInside []southbound.CGNATInsidePrefixState,
	actualOutside []southbound.CGNATOutsideAddressState, vr vrfResolver,
	rc *cgnat.ReconcileConfig) error {

	if rc.GetAllowPoolDisruption() {
		return nil
	}

	var disruption []string
	for _, plan := range plans {
		switch plan.action {
		case poolActionReplace:
			if plan.activeMappings > 0 {
				disruption = append(disruption,
					fmt.Sprintf("pool %q: hard-drift replace (drift=%v) would drop %d active mapping(s)",
						plan.name, plan.driftFields, plan.activeMappings))
			}
		case poolActionDrop:
			if !rc.GetDropOrphans() {
				continue
			}
			if plan.activeMappings > 0 {
				disruption = append(disruption,
					fmt.Sprintf("orphan pool id=%d: drop would discard %d active mapping(s)",
						plan.poolID, plan.activeMappings))
			}
		}
	}

	desiredInside := buildDesiredInside(desiredPools, desiredIDByName, vr)
	for _, cur := range actualInside {
		k := insidePrefixKey{poolID: cur.PoolID, prefix: cur.Prefix.String(), vrfID: cur.VRFID}
		if _, want := desiredInside[k]; want {
			continue
		}
		if rc.GetDropOrphans() && hasActiveMappingsUnder(plans, cur.PoolID) {
			disruption = append(disruption,
				fmt.Sprintf("inside prefix %s vrf=%d (pool id=%d) removal would drop mappings",
					cur.Prefix.String(), cur.VRFID, cur.PoolID))
		}
	}

	desiredOutside := buildDesiredOutside(desiredPools, desiredIDByName)
	for _, cur := range actualOutside {
		k := outsideAddressKey{poolID: cur.PoolID, prefix: cur.Prefix.String()}
		if _, want := desiredOutside[k]; want {
			continue
		}
		if rc.GetDropOrphans() && hasActiveMappingsUnder(plans, cur.PoolID) {
			disruption = append(disruption,
				fmt.Sprintf("outside address %s (pool id=%d) removal would drop mappings",
					cur.Prefix.String(), cur.PoolID))
		}
	}

	if len(disruption) > 0 {
		msg := "cgnat: reconcile: refusing to disrupt active subscriber NAT state. "
		msg += "Set cgnat.reconcile.allow_pool_disruption: true to proceed, OR schedule a maintenance window. Affected:\n"
		for _, d := range disruption {
			msg += "  - " + d + "\n"
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func buildDesiredInside(desiredPools map[string]*cgnat.Pool, desiredIDByName map[string]uint32, vr vrfResolver) map[insidePrefixKey]struct{} {
	out := make(map[insidePrefixKey]struct{})
	for name, p := range desiredPools {
		if p == nil {
			continue
		}
		id := desiredIDByName[name]
		for _, ip := range p.InsidePrefixes {
			_, ipNet, err := net.ParseCIDR(ip.Prefix)
			if err != nil {
				continue
			}
			vrfID := uint32(0)
			if ip.VRF != "" && vr != nil {
				if tid, _, _, err := vr.ResolveVRF(ip.VRF); err == nil {
					vrfID = tid
				}
			}
			out[insidePrefixKey{poolID: id, prefix: ipNet.String(), vrfID: vrfID}] = struct{}{}
		}
	}
	return out
}

func buildDesiredOutside(desiredPools map[string]*cgnat.Pool, desiredIDByName map[string]uint32) map[outsideAddressKey]struct{} {
	out := make(map[outsideAddressKey]struct{})
	for name, p := range desiredPools {
		if p == nil {
			continue
		}
		id := desiredIDByName[name]
		for _, addr := range p.OutsideAddresses {
			n := parseOutsideAddr(addr)
			if n == nil {
				continue
			}
			out[outsideAddressKey{poolID: id, prefix: n.String()}] = struct{}{}
		}
	}
	return out
}

func parseOutsideAddr(s string) *net.IPNet {
	if _, n, err := net.ParseCIDR(s); err == nil {
		return n
	}
	if ip := net.ParseIP(s); ip != nil {
		return &net.IPNet{IP: ip.To4(), Mask: net.CIDRMask(32, 32)}
	}
	return nil
}

func hasActiveMappingsUnder(plans []poolPlan, poolID uint32) bool {
	for _, p := range plans {
		if p.poolID == poolID && p.activeMappings > 0 {
			return true
		}
	}
	return false
}

func reconcileChildren(deps reconcileDeps, plans []poolPlan, desiredPools map[string]*cgnat.Pool,
	desiredIDByName map[string]uint32, actualInside []southbound.CGNATInsidePrefixState,
	actualOutside []southbound.CGNATOutsideAddressState, rc *cgnat.ReconcileConfig) error {

	replacedOrAddedPool := make(map[uint32]bool)
	for _, plan := range plans {
		if plan.action == poolActionAdd || plan.action == poolActionReplace {
			replacedOrAddedPool[plan.poolID] = true
		}
	}

	desiredInside := buildDesiredInside(desiredPools, desiredIDByName, deps.vrf)
	actualInsideSet := make(map[insidePrefixKey]net.IPNet, len(actualInside))
	for _, e := range actualInside {
		k := insidePrefixKey{poolID: e.PoolID, prefix: e.Prefix.String(), vrfID: e.VRFID}
		actualInsideSet[k] = e.Prefix
	}

	for k := range desiredInside {
		if replacedOrAddedPool[k.poolID] {
			if err := applyInsidePrefix(deps.dp, k); err != nil {
				return err
			}
			continue
		}
		if _, exists := actualInsideSet[k]; !exists {
			if err := applyInsidePrefix(deps.dp, k); err != nil {
				return err
			}
			if deps.log != nil {
				deps.log.Info("cgnat reconcile: add inside-prefix", "pool_id", k.poolID, "prefix", k.prefix, "vrf", k.vrfID)
			}
		}
	}
	if rc.GetDropOrphans() {
		for k, pfx := range actualInsideSet {
			if _, want := desiredInside[k]; want {
				continue
			}
			if replacedOrAddedPool[k.poolID] {
				continue
			}
			if err := deps.dp.CGNATPoolAddInsidePrefix(k.poolID, pfx, k.vrfID, false); err != nil {
				return fmt.Errorf("drop orphan inside-prefix pool=%d %s: %w", k.poolID, pfx.String(), err)
			}
			if deps.log != nil {
				deps.log.Warn("cgnat reconcile: drop orphan inside-prefix", "pool_id", k.poolID, "prefix", pfx.String(), "vrf", k.vrfID)
			}
		}
	}

	desiredOutside := buildDesiredOutside(desiredPools, desiredIDByName)
	actualOutsideSet := make(map[outsideAddressKey]net.IPNet, len(actualOutside))
	for _, e := range actualOutside {
		k := outsideAddressKey{poolID: e.PoolID, prefix: e.Prefix.String()}
		actualOutsideSet[k] = e.Prefix
	}
	for k := range desiredOutside {
		if replacedOrAddedPool[k.poolID] {
			if err := applyOutsideAddress(deps.dp, k); err != nil {
				return err
			}
			continue
		}
		if _, exists := actualOutsideSet[k]; !exists {
			if err := applyOutsideAddress(deps.dp, k); err != nil {
				return err
			}
			if deps.log != nil {
				deps.log.Info("cgnat reconcile: add outside-address", "pool_id", k.poolID, "prefix", k.prefix)
			}
		}
	}
	if rc.GetDropOrphans() {
		for k, pfx := range actualOutsideSet {
			if _, want := desiredOutside[k]; want {
				continue
			}
			if replacedOrAddedPool[k.poolID] {
				continue
			}
			if err := deps.dp.CGNATPoolAddOutsideAddress(k.poolID, pfx, false); err != nil {
				return fmt.Errorf("drop orphan outside-address pool=%d %s: %w", k.poolID, pfx.String(), err)
			}
			if deps.log != nil {
				deps.log.Warn("cgnat reconcile: drop orphan outside-address", "pool_id", k.poolID, "prefix", pfx.String())
			}
		}
	}
	return nil
}

func applyInsidePrefix(dp southbound.CGNATDataplane, k insidePrefixKey) error {
	_, n, err := net.ParseCIDR(k.prefix)
	if err != nil {
		return fmt.Errorf("parse inside prefix %q: %w", k.prefix, err)
	}
	return dp.CGNATPoolAddInsidePrefix(k.poolID, *n, k.vrfID, true)
}

func applyOutsideAddress(dp southbound.CGNATDataplane, k outsideAddressKey) error {
	_, n, err := net.ParseCIDR(k.prefix)
	if err != nil {
		return fmt.Errorf("parse outside address %q: %w", k.prefix, err)
	}
	return dp.CGNATPoolAddOutsideAddress(k.poolID, *n, true)
}

func verify(dp southbound.CGNATDataplane, plans []poolPlan, desiredPools map[string]*cgnat.Pool, desiredIDByName map[string]uint32) error {
	got, err := dp.CGNATPoolDump()
	if err != nil {
		return fmt.Errorf("verify dump: %w", err)
	}
	gotByID := make(map[uint32]*southbound.CGNATPoolState, len(got))
	for i := range got {
		gotByID[got[i].PoolID] = &got[i]
	}
	for name, id := range desiredIDByName {
		cur, ok := gotByID[id]
		if !ok {
			return fmt.Errorf("pool %q (id=%d) missing post-apply", name, id)
		}
		if hard, fields := poolHardDrift(cur, desiredPools[name]); hard {
			return fmt.Errorf("pool %q (id=%d) still hard-drifted post-apply: %v", name, id, fields)
		}
	}
	return nil
}

func populateLocalState(deps reconcileDeps, desiredPools map[string]*cgnat.Pool, desiredIDByName map[string]uint32) {
	for name, p := range desiredPools {
		if p == nil {
			continue
		}
		id := desiredIDByName[name]
		deps.poolIDMap[name] = id
		if deps.pools != nil {
			_ = deps.pools.ConfigurePool(name, id, p)
		}
		if deps.blacklist != nil {
			for _, excluded := range p.ExcludedAddresses {
				ip := net.ParseIP(excluded)
				if ip != nil {
					deps.blacklist.Exclude(name, ip)
				}
			}
		}
	}
}
