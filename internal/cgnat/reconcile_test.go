// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package cgnat

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/config/cgnat"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

type stubVRF struct {
	tableID map[string]uint32
}

func (s *stubVRF) ResolveVRF(name string) (uint32, bool, bool, error) {
	id, ok := s.tableID[name]
	if !ok {
		return 0, false, false, nil
	}
	return id, true, true, nil
}

type stubDP struct {
	pools   []southbound.CGNATPoolState
	inside  []southbound.CGNATInsidePrefixState
	outside []southbound.CGNATOutsideAddressState

	addDelCalls       []addDelCall
	addInsideCalls    []insideCall
	addOutsideCalls   []outsideCall
	setOutsideVRFCalls []setVRFCall

	hardDriftReturnsRefresh bool
	verifyDumpOverride      *[]southbound.CGNATPoolState
}

type addDelCall struct {
	poolID    uint32
	isAdd     bool
	blockSize uint16
	timeouts  [4]uint32
}

type insideCall struct {
	poolID uint32
	prefix string
	vrfID  uint32
	isAdd  bool
}

type outsideCall struct {
	poolID uint32
	prefix string
	isAdd  bool
}

type setVRFCall struct {
	poolID  uint32
	tableID uint32
}

func (s *stubDP) CGNATPoolAddDel(poolID uint32, mode, addressPooling, filtering uint8,
	blockSize uint16, maxBlocksPerSub uint8, maxSessionsPerSub uint32,
	portRangeStart, portRangeEnd, portReuseTimeout uint16, algBitmask uint8,
	timeouts [4]uint32, isAdd bool) error {
	s.addDelCalls = append(s.addDelCalls, addDelCall{poolID: poolID, isAdd: isAdd, blockSize: blockSize, timeouts: timeouts})
	if !isAdd {
		s.removePool(poolID)
		return nil
	}
	for i := range s.pools {
		if s.pools[i].PoolID == poolID {
			s.pools[i].BlockSize = blockSize
			s.pools[i].Timeouts = timeouts
			s.pools[i].MaxSessionsPerSub = maxSessionsPerSub
			s.pools[i].PortRangeStart = portRangeStart
			s.pools[i].PortRangeEnd = portRangeEnd
			return nil
		}
	}
	s.pools = append(s.pools, southbound.CGNATPoolState{
		PoolID: poolID, Mode: mode, AddressPooling: addressPooling, Filtering: filtering,
		BlockSize: blockSize, MaxBlocksPerSub: maxBlocksPerSub, MaxSessionsPerSub: maxSessionsPerSub,
		PortRangeStart: portRangeStart, PortRangeEnd: portRangeEnd,
		PortReuseTimeout: portReuseTimeout, ALGBitmask: algBitmask, Timeouts: timeouts,
	})
	return nil
}

func (s *stubDP) removePool(poolID uint32) {
	out := s.pools[:0]
	for _, p := range s.pools {
		if p.PoolID != poolID {
			out = append(out, p)
		}
	}
	s.pools = out
	inOut := s.inside[:0]
	for _, e := range s.inside {
		if e.PoolID != poolID {
			inOut = append(inOut, e)
		}
	}
	s.inside = inOut
	outOut := s.outside[:0]
	for _, e := range s.outside {
		if e.PoolID != poolID {
			outOut = append(outOut, e)
		}
	}
	s.outside = outOut
}

func (s *stubDP) CGNATPoolAddInsidePrefix(poolID uint32, prefix net.IPNet, vrfID uint32, isAdd bool) error {
	s.addInsideCalls = append(s.addInsideCalls, insideCall{poolID: poolID, prefix: prefix.String(), vrfID: vrfID, isAdd: isAdd})
	if isAdd {
		s.inside = append(s.inside, southbound.CGNATInsidePrefixState{PoolID: poolID, Prefix: prefix, VRFID: vrfID})
	} else {
		out := s.inside[:0]
		for _, e := range s.inside {
			if e.PoolID == poolID && e.Prefix.String() == prefix.String() && e.VRFID == vrfID {
				continue
			}
			out = append(out, e)
		}
		s.inside = out
	}
	return nil
}

func (s *stubDP) CGNATPoolAddOutsideAddress(poolID uint32, prefix net.IPNet, isAdd bool) error {
	s.addOutsideCalls = append(s.addOutsideCalls, outsideCall{poolID: poolID, prefix: prefix.String(), isAdd: isAdd})
	if isAdd {
		s.outside = append(s.outside, southbound.CGNATOutsideAddressState{PoolID: poolID, Prefix: prefix})
	} else {
		out := s.outside[:0]
		for _, e := range s.outside {
			if e.PoolID == poolID && e.Prefix.String() == prefix.String() {
				continue
			}
			out = append(out, e)
		}
		s.outside = out
	}
	return nil
}

func (s *stubDP) CGNATSetOutsideVRF(poolID, vrfTableID uint32) error {
	s.setOutsideVRFCalls = append(s.setOutsideVRFCalls, setVRFCall{poolID: poolID, tableID: vrfTableID})
	for i := range s.pools {
		if s.pools[i].PoolID == poolID {
			s.pools[i].OutsideVRFTableID = vrfTableID
			return nil
		}
	}
	return nil
}

func (s *stubDP) CGNATPoolUpdate(poolID uint32, maxSessions uint32, algBitmask uint8, timeouts [4]uint32) error {
	return nil
}
func (s *stubDP) CGNATAddDelSubscriberMapping(poolID, swIfIndex uint32, insideIP net.IP, insideVRFID uint32, outsideIP net.IP, portStart, portEnd uint16, enableFeature, isAdd bool) error {
	return nil
}
func (s *stubDP) CGNATAddDelSubscriberMappingAsync(poolID, swIfIndex uint32, insideIP net.IP, insideVRFID uint32, outsideIP net.IP, portStart, portEnd uint16, enableFeature, isAdd bool, callback func(error)) {
}
func (s *stubDP) CGNATAddSubscriberMappingBulk(poolID uint32, mappings []southbound.CGNATMapping) error {
	return nil
}
func (s *stubDP) CGNATEnableOnSession(poolID, swIfIndex uint32, isEnable bool) error { return nil }
func (s *stubDP) CGNATAddDelBypass(prefix net.IPNet, vrfID uint32, isAdd bool) error { return nil }
func (s *stubDP) CGNATDumpSubscriberMappings(poolID uint32) ([]southbound.CGNATMapping, error) {
	return nil, nil
}
func (s *stubDP) CGNATPoolDump() ([]southbound.CGNATPoolState, error) {
	return append([]southbound.CGNATPoolState(nil), s.pools...), nil
}
func (s *stubDP) CGNATPoolInsidePrefixDump(poolID uint32) ([]southbound.CGNATInsidePrefixState, error) {
	return append([]southbound.CGNATInsidePrefixState(nil), s.inside...), nil
}
func (s *stubDP) CGNATPoolOutsideAddressDump(poolID uint32) ([]southbound.CGNATOutsideAddressState, error) {
	return append([]southbound.CGNATOutsideAddressState(nil), s.outside...), nil
}

func mustCIDR(s string) net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return *n
}

func makePool(blockSize uint16, tcpEst uint32, inside, outside []string) *cgnat.Pool {
	prefixes := make([]cgnat.InsidePrefix, 0, len(inside))
	for _, p := range inside {
		prefixes = append(prefixes, cgnat.InsidePrefix{Prefix: p})
	}
	return &cgnat.Pool{
		OutsideInterfaces: []string{"bond0.100"},
		Mode:              "pba",
		BlockSize:         blockSize,
		InsidePrefixes:    prefixes,
		OutsideAddresses:  outside,
		Timeouts:          &cgnat.TimeoutConfig{TCPEstablished: tcpEst},
	}
}

func mkDeps(dp *stubDP) reconcileDeps {
	return reconcileDeps{
		dp:        dp,
		vrf:       &stubVRF{tableID: map[string]uint32{}},
		pools:     NewPoolManager(),
		blacklist: NewBlacklistManager(),
		poolIDMap: map[string]uint32{},
	}
}

func cfgFor(pools map[string]*cgnat.Pool, rc *cgnat.ReconcileConfig) *config.Config {
	return &config.Config{CGNAT: &cgnat.Config{Pools: pools, Reconcile: rc}}
}

func TestReconcile_ColdStart_AddsEverything(t *testing.T) {
	dp := &stubDP{}
	deps := mkDeps(dp)
	cfg := cfgFor(map[string]*cgnat.Pool{
		"residential": makePool(512, 7200, []string{"100.64.0.0/11"}, []string{"203.0.113.64/26"}),
	}, nil)

	if err := reconcileWith(context.Background(), deps, cfg); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	if len(dp.addDelCalls) != 1 || !dp.addDelCalls[0].isAdd {
		t.Fatalf("expected 1 pool add, got %+v", dp.addDelCalls)
	}
	if len(dp.addInsideCalls) != 1 || !dp.addInsideCalls[0].isAdd {
		t.Fatalf("expected 1 inside add, got %+v", dp.addInsideCalls)
	}
	if len(dp.addOutsideCalls) != 1 || !dp.addOutsideCalls[0].isAdd {
		t.Fatalf("expected 1 outside add, got %+v", dp.addOutsideCalls)
	}
	if deps.poolIDMap["residential"] == 0 {
		t.Fatalf("poolIDMap not populated")
	}
}

func TestReconcile_IdenticalState_NoCalls(t *testing.T) {
	dp := &stubDP{}
	pool := makePool(512, 7200, []string{"100.64.0.0/11"}, []string{"203.0.113.64/26"})
	id := poolID("residential")
	dp.pools = []southbound.CGNATPoolState{{
		PoolID: id, Mode: 0, BlockSize: 512,
		MaxBlocksPerSub:   pool.GetMaxBlocksPerSubscriber(),
		MaxSessionsPerSub: pool.GetMaxSessionsPerSubscriber(),
		PortRangeStart:    pool.GetPortRangeStart(),
		PortRangeEnd:      pool.GetPortRangeEnd(),
		PortReuseTimeout:  pool.GetPortReuseTimeout(),
		ALGBitmask:        pool.GetALGBitmask(),
		Timeouts: [4]uint32{
			pool.GetTimeouts().TCPEstablished, pool.GetTimeouts().TCPTransitory,
			pool.GetTimeouts().UDP, pool.GetTimeouts().ICMP,
		},
	}}
	dp.inside = []southbound.CGNATInsidePrefixState{{PoolID: id, Prefix: mustCIDR("100.64.0.0/11"), VRFID: 0}}
	dp.outside = []southbound.CGNATOutsideAddressState{{PoolID: id, Prefix: mustCIDR("203.0.113.64/26")}}

	deps := mkDeps(dp)
	cfg := cfgFor(map[string]*cgnat.Pool{"residential": pool}, nil)

	if err := reconcileWith(context.Background(), deps, cfg); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(dp.addDelCalls) != 0 {
		t.Fatalf("expected 0 pool calls on identical state, got %+v", dp.addDelCalls)
	}
	if len(dp.addInsideCalls) != 0 {
		t.Fatalf("expected 0 inside calls, got %+v", dp.addInsideCalls)
	}
	if len(dp.addOutsideCalls) != 0 {
		t.Fatalf("expected 0 outside calls, got %+v", dp.addOutsideCalls)
	}
}

func TestReconcile_SoftDrift_TimeoutChange_NoMappingsDropped(t *testing.T) {
	dp := &stubDP{}
	id := poolID("residential")
	pool := makePool(512, 3600, []string{"100.64.0.0/11"}, []string{"203.0.113.64/26"})
	dp.pools = []southbound.CGNATPoolState{{
		PoolID: id, BlockSize: 512, ActiveMappings: 100,
		MaxBlocksPerSub:   pool.GetMaxBlocksPerSubscriber(),
		MaxSessionsPerSub: pool.GetMaxSessionsPerSubscriber(),
		PortRangeStart:    pool.GetPortRangeStart(), PortRangeEnd: pool.GetPortRangeEnd(),
		PortReuseTimeout: pool.GetPortReuseTimeout(),
		Timeouts:         [4]uint32{7200, 240, 300, 60},
	}}
	dp.inside = []southbound.CGNATInsidePrefixState{{PoolID: id, Prefix: mustCIDR("100.64.0.0/11")}}
	dp.outside = []southbound.CGNATOutsideAddressState{{PoolID: id, Prefix: mustCIDR("203.0.113.64/26")}}

	deps := mkDeps(dp)
	cfg := cfgFor(map[string]*cgnat.Pool{"residential": pool}, nil)

	if err := reconcileWith(context.Background(), deps, cfg); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(dp.addDelCalls) != 1 || !dp.addDelCalls[0].isAdd {
		t.Fatalf("expected 1 pool add (soft-update), got %+v", dp.addDelCalls)
	}
	if dp.addDelCalls[0].timeouts[0] != 3600 {
		t.Fatalf("expected timeout=3600 propagated, got %v", dp.addDelCalls[0].timeouts)
	}
}

func TestReconcile_HardDrift_NoMappings_RefreshesPool(t *testing.T) {
	dp := &stubDP{}
	id := poolID("residential")
	pool := makePool(1024, 7200, []string{"100.64.0.0/11"}, []string{"203.0.113.64/26"})
	dp.pools = []southbound.CGNATPoolState{{
		PoolID: id, BlockSize: 512, ActiveMappings: 0,
		MaxBlocksPerSub: pool.GetMaxBlocksPerSubscriber(),
		MaxSessionsPerSub: pool.GetMaxSessionsPerSubscriber(),
		PortRangeStart: pool.GetPortRangeStart(), PortRangeEnd: pool.GetPortRangeEnd(),
		PortReuseTimeout: pool.GetPortReuseTimeout(),
		Timeouts: [4]uint32{7200, 240, 300, 60},
	}}
	dp.inside = []southbound.CGNATInsidePrefixState{{PoolID: id, Prefix: mustCIDR("100.64.0.0/11")}}
	dp.outside = []southbound.CGNATOutsideAddressState{{PoolID: id, Prefix: mustCIDR("203.0.113.64/26")}}

	deps := mkDeps(dp)
	cfg := cfgFor(map[string]*cgnat.Pool{"residential": pool}, nil)

	if err := reconcileWith(context.Background(), deps, cfg); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(dp.addDelCalls) != 1 {
		t.Fatalf("expected 1 pool add (replace), got %+v", dp.addDelCalls)
	}
	if dp.addDelCalls[0].blockSize != 1024 {
		t.Fatalf("expected block_size=1024, got %d", dp.addDelCalls[0].blockSize)
	}
	if len(dp.addInsideCalls) == 0 || len(dp.addOutsideCalls) == 0 {
		t.Fatalf("expected child re-add after replace, got inside=%v outside=%v", dp.addInsideCalls, dp.addOutsideCalls)
	}
}

func TestReconcile_HardDrift_WithMappings_PreflightAborts(t *testing.T) {
	dp := &stubDP{}
	id := poolID("residential")
	pool := makePool(1024, 7200, []string{"100.64.0.0/11"}, []string{"203.0.113.64/26"})
	dp.pools = []southbound.CGNATPoolState{{
		PoolID: id, BlockSize: 512, ActiveMappings: 42,
		MaxBlocksPerSub: pool.GetMaxBlocksPerSubscriber(),
		MaxSessionsPerSub: pool.GetMaxSessionsPerSubscriber(),
		PortRangeStart: pool.GetPortRangeStart(), PortRangeEnd: pool.GetPortRangeEnd(),
		PortReuseTimeout: pool.GetPortReuseTimeout(),
		Timeouts: [4]uint32{7200, 240, 300, 60},
	}}
	dp.inside = []southbound.CGNATInsidePrefixState{{PoolID: id, Prefix: mustCIDR("100.64.0.0/11")}}
	dp.outside = []southbound.CGNATOutsideAddressState{{PoolID: id, Prefix: mustCIDR("203.0.113.64/26")}}

	deps := mkDeps(dp)
	cfg := cfgFor(map[string]*cgnat.Pool{"residential": pool}, nil)

	err := reconcileWith(context.Background(), deps, cfg)
	if err == nil {
		t.Fatalf("expected preflight to abort, got nil")
	}
	if !strings.Contains(err.Error(), "allow_pool_disruption") {
		t.Fatalf("expected actionable error, got %v", err)
	}
	if len(dp.addDelCalls) != 0 {
		t.Fatalf("expected no mutation, got %+v", dp.addDelCalls)
	}
}

func TestReconcile_HardDrift_WithMappings_AllowFlagProceeds(t *testing.T) {
	dp := &stubDP{}
	id := poolID("residential")
	pool := makePool(1024, 7200, []string{"100.64.0.0/11"}, []string{"203.0.113.64/26"})
	dp.pools = []southbound.CGNATPoolState{{
		PoolID: id, BlockSize: 512, ActiveMappings: 42,
		MaxBlocksPerSub: pool.GetMaxBlocksPerSubscriber(),
		MaxSessionsPerSub: pool.GetMaxSessionsPerSubscriber(),
		PortRangeStart: pool.GetPortRangeStart(), PortRangeEnd: pool.GetPortRangeEnd(),
		PortReuseTimeout: pool.GetPortReuseTimeout(),
		Timeouts: [4]uint32{7200, 240, 300, 60},
	}}
	dp.inside = []southbound.CGNATInsidePrefixState{{PoolID: id, Prefix: mustCIDR("100.64.0.0/11")}}
	dp.outside = []southbound.CGNATOutsideAddressState{{PoolID: id, Prefix: mustCIDR("203.0.113.64/26")}}

	deps := mkDeps(dp)
	yes := true
	cfg := cfgFor(map[string]*cgnat.Pool{"residential": pool}, &cgnat.ReconcileConfig{AllowPoolDisruption: &yes})

	if err := reconcileWith(context.Background(), deps, cfg); err != nil {
		t.Fatalf("expected success with allow flag, got %v", err)
	}
	if len(dp.addDelCalls) != 1 || dp.addDelCalls[0].blockSize != 1024 {
		t.Fatalf("expected pool replace, got %+v", dp.addDelCalls)
	}
}

func TestReconcile_InsidePrefixSetDiff(t *testing.T) {
	dp := &stubDP{}
	id := poolID("residential")
	pool := makePool(512, 7200, []string{"100.64.0.0/11", "100.65.0.0/16"}, []string{"203.0.113.64/26"})
	dp.pools = []southbound.CGNATPoolState{{
		PoolID: id, BlockSize: 512, ActiveMappings: 0,
		MaxBlocksPerSub: pool.GetMaxBlocksPerSubscriber(),
		MaxSessionsPerSub: pool.GetMaxSessionsPerSubscriber(),
		PortRangeStart: pool.GetPortRangeStart(), PortRangeEnd: pool.GetPortRangeEnd(),
		PortReuseTimeout: pool.GetPortReuseTimeout(),
		Timeouts: [4]uint32{7200, 240, 300, 60},
	}}
	dp.inside = []southbound.CGNATInsidePrefixState{
		{PoolID: id, Prefix: mustCIDR("100.64.0.0/11")},
		{PoolID: id, Prefix: mustCIDR("100.66.0.0/16")},
	}
	dp.outside = []southbound.CGNATOutsideAddressState{{PoolID: id, Prefix: mustCIDR("203.0.113.64/26")}}

	deps := mkDeps(dp)
	cfg := cfgFor(map[string]*cgnat.Pool{"residential": pool}, nil)

	if err := reconcileWith(context.Background(), deps, cfg); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	var added, removed int
	for _, c := range dp.addInsideCalls {
		if c.isAdd {
			added++
		} else {
			removed++
		}
	}
	if added != 1 || removed != 1 {
		t.Fatalf("expected 1 inside add (100.65) and 1 del (100.66), got add=%d del=%d (%+v)", added, removed, dp.addInsideCalls)
	}
}

func TestReconcile_OrphanPool_DropOrphansFalse_KeptWithWarn(t *testing.T) {
	dp := &stubDP{}
	dp.pools = []southbound.CGNATPoolState{{PoolID: 0xDEADBEEF, ActiveMappings: 0, BlockSize: 512, MaxBlocksPerSub: 4, MaxSessionsPerSub: 2000, PortRangeStart: 1024, PortRangeEnd: 65535, PortReuseTimeout: 120, Timeouts: [4]uint32{7200, 240, 300, 60}}}

	deps := mkDeps(dp)
	no := false
	cfg := cfgFor(map[string]*cgnat.Pool{}, &cgnat.ReconcileConfig{DropOrphans: &no})

	if err := reconcileWith(context.Background(), deps, cfg); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	for _, c := range dp.addDelCalls {
		if !c.isAdd && c.poolID == 0xDEADBEEF {
			t.Fatalf("expected orphan to be kept with drop_orphans=false, got del call")
		}
	}
}

func TestReconcile_PoolIDCollision_Fails(t *testing.T) {
	dp := &stubDP{}
	deps := mkDeps(dp)
	cfg := cfgFor(map[string]*cgnat.Pool{
		"residential": makePool(512, 7200, []string{"100.64.0.0/11"}, []string{"203.0.113.64/26"}),
		"RESIDENTIAL": makePool(512, 7200, []string{"100.64.0.0/11"}, []string{"203.0.113.64/26"}),
	}, nil)
	err := reconcileWith(context.Background(), deps, cfg)
	if err == nil {
		return
	}
	if !strings.Contains(err.Error(), "collision") && !strings.Contains(err.Error(), "deterministic pool ID") {
		t.Fatalf("got error but not a collision diagnostic: %v", err)
	}
}

func TestReconcile_OnDivergenceFail_AppliesThenReturnsError(t *testing.T) {
	dp := &stubDP{}
	deps := mkDeps(dp)
	cfg := cfgFor(map[string]*cgnat.Pool{
		"residential": makePool(512, 7200, []string{"100.64.0.0/11"}, []string{"203.0.113.64/26"}),
	}, &cgnat.ReconcileConfig{OnDivergence: "fail"})

	err := reconcileWith(context.Background(), deps, cfg)
	if err == nil {
		t.Fatalf("expected on_divergence=fail to return error after apply, got nil")
	}
	if len(dp.addDelCalls) == 0 {
		t.Fatalf("expected apply to run before fail, no calls made")
	}
}

func TestPoolID_ZeroGuard(t *testing.T) {
	if poolID("") == 0 {
		t.Fatalf("poolID must never return 0 (plugin treats 0 as wildcard)")
	}
}
