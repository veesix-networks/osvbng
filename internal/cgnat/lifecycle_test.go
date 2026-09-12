// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package cgnat

import (
	"context"
	"errors"
	"math/rand"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/veesix-networks/osvbng/pkg/models"
)

type asyncCall struct {
	swIfIndex uint32
	vrfID     uint32
	outsideIP net.IP
	portStart uint16
	isAdd     bool
	done      func(error)
}

// parkedDP records mapping calls and holds every callback until the test
// completes it, so the interleaving under test is chosen by the test and
// not by the scheduler. inflight counts callbacks not yet completed; the
// writer must never let it exceed one per key.
type parkedDP struct {
	stubDP
	mu          sync.Mutex
	calls       []asyncCall
	completed   []bool
	inline      bool
	inflight    int
	maxInflight int
	bypasses    []bypassCall
}

func (d *parkedDP) CGNATAddDelSubscriberMappingAsync(poolID, swIfIndex uint32, insideIP net.IP, insideVRFID uint32, outsideIP net.IP, portStart, portEnd uint16, enableFeature, isAdd bool, callback func(error)) {
	d.mu.Lock()
	d.inflight++
	if d.inflight > d.maxInflight {
		d.maxInflight = d.inflight
	}
	idx := len(d.calls)
	var once sync.Once
	call := asyncCall{swIfIndex: swIfIndex, vrfID: insideVRFID, outsideIP: outsideIP, portStart: portStart, isAdd: isAdd}
	call.done = func(err error) {
		once.Do(func() {
			d.mu.Lock()
			d.inflight--
			d.completed[idx] = true
			d.mu.Unlock()
			callback(err)
		})
	}
	d.calls = append(d.calls, call)
	d.completed = append(d.completed, false)
	inline := d.inline
	d.mu.Unlock()
	if inline {
		call.done(nil)
	}
}

func (d *parkedDP) CGNATAddDelBypass(prefix net.IPNet, vrfID uint32, isAdd bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.bypasses = append(d.bypasses, bypassCall{vrfID: vrfID, isAdd: isAdd})
	return nil
}

func (d *parkedDP) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.calls)
}

func (d *parkedDP) call(i int) asyncCall {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls[i]
}

func (d *parkedDP) complete(i int, err error) {
	d.call(i).done(err)
}

// programmed reports the session the writer has programmed for the key.
func (c *Component) programmed(ip string, vrf uint32) (sessionID, pool string) {
	key := makeSubscriberKey(vrf, net.ParseIP(ip))
	c.subMu.Lock()
	defer c.subMu.Unlock()
	st := c.subs[key]
	if st == nil || st.have == nil {
		return "", ""
	}
	return st.have.sessionID, st.have.class.poolName
}

func (c *Component) opdbKeys(t *testing.T) []string {
	t.Helper()
	var keys []string
	if err := c.opdb.Load(context.Background(), opdbNamespace, func(key string, _ []byte) error {
		keys = append(keys, key)
		return nil
	}); err != nil {
		t.Fatalf("load opdb: %v", err)
	}
	return keys
}

func blocksHeld(c *Component, ip string, vrf uint32) int {
	return len(c.pools.GetMappings("p1", net.ParseIP(ip).To4(), vrf))
}

func TestLifecycle_NewSessionTakesOverKey_OldReleaseIsNoOp(t *testing.T) {
	dp := &parkedDP{inline: true}
	c := newVRFComponent(t, dp)

	c.handleSessionActivate(ipoeEvent("a1", "100.64.0.2", "CUSTOMER-A", "nat", models.SessionStateActive))
	ev := ipoeEvent("a2", "100.64.0.2", "CUSTOMER-A", "nat", models.SessionStateActive)
	ev.Session.(*models.IPoESession).IfIndex = 9
	c.handleSessionActivate(ev)
	c.handleSessionRelease(ipoeEvent("a1", "100.64.0.2", "CUSTOMER-A", "nat", models.SessionStateReleased))

	if dp.count() != 2 || !dp.call(0).isAdd || !dp.call(1).isAdd {
		t.Fatalf("expected add(a1), add(a2) and no delete for a1's release, got %d calls", dp.count())
	}
	if dp.call(1).swIfIndex != 9 || dp.call(0).portStart != dp.call(1).portStart {
		t.Fatalf("a2 must refresh the same block onto its interface: %+v %+v", dp.call(0), dp.call(1))
	}
	if sid, _ := c.programmed("100.64.0.2", 100); sid != "a2" {
		t.Fatalf("a2 must hold the key after a1's release, got %q", sid)
	}
	if got := blocksHeld(c, "100.64.0.2", 100); got != 1 {
		t.Fatalf("exactly one block must be held, got %d", got)
	}
	if keys := c.opdbKeys(t); len(keys) != 1 || keys[0] != "a2" {
		t.Fatalf("opdb must record the holder only, got %v", keys)
	}
	if m := c.reverse.Lookup(dp.call(1).outsideIP, dp.call(1).portStart); m == nil || m.SessionID != "a2" {
		t.Fatalf("reverse index must point at a2, got %+v", m)
	}

	c.handleSessionRelease(ipoeEvent("a2", "100.64.0.2", "CUSTOMER-A", "nat", models.SessionStateReleased))

	if dp.count() != 3 || dp.call(2).isAdd {
		t.Fatalf("the holder's release must delete the mapping, got %d calls", dp.count())
	}
	if got := blocksHeld(c, "100.64.0.2", 100); got != 0 {
		t.Fatalf("block must be freed after the holder's release, got %d", got)
	}
	if keys := c.opdbKeys(t); len(keys) != 0 {
		t.Fatalf("opdb must be empty after the holder's release, got %v", keys)
	}
}

func TestLifecycle_ActivateWaitsForInFlightDelete(t *testing.T) {
	dp := &parkedDP{}
	c := newVRFComponent(t, dp)

	c.handleSessionActivate(ipoeEvent("a1", "100.64.0.2", "CUSTOMER-A", "nat", models.SessionStateActive))
	dp.complete(0, nil)
	c.handleSessionRelease(ipoeEvent("a1", "100.64.0.2", "CUSTOMER-A", "nat", models.SessionStateReleased))
	if dp.count() != 2 || dp.call(1).isAdd {
		t.Fatalf("expected a parked delete, got %d calls", dp.count())
	}

	c.handleSessionActivate(ipoeEvent("a2", "100.64.0.2", "CUSTOMER-A", "nat", models.SessionStateActive))
	if dp.count() != 2 {
		t.Fatalf("a2's add must wait for a1's delete, got %d calls", dp.count())
	}

	dp.complete(1, nil)
	if dp.count() != 3 || !dp.call(2).isAdd {
		t.Fatalf("a2's add must follow the completed delete, got %d calls", dp.count())
	}
	dp.complete(2, nil)

	if sid, _ := c.programmed("100.64.0.2", 100); sid != "a2" {
		t.Fatalf("a2 must hold the key, got %q", sid)
	}
	if got := blocksHeld(c, "100.64.0.2", 100); got != 1 {
		t.Fatalf("exactly one block must be held, got %d", got)
	}
	if keys := c.opdbKeys(t); len(keys) != 1 || keys[0] != "a2" {
		t.Fatalf("opdb must record a2 only, got %v", keys)
	}
	if dp.maxInflight != 1 {
		t.Fatalf("one dataplane call in flight per key, saw %d", dp.maxInflight)
	}
}

func TestLifecycle_ReleaseDuringInFlightAddTearsDownAfter(t *testing.T) {
	dp := &parkedDP{}
	c := newVRFComponent(t, dp)

	c.handleSessionActivate(ipoeEvent("a1", "100.64.0.2", "CUSTOMER-A", "nat", models.SessionStateActive))
	c.handleSessionRelease(ipoeEvent("a1", "100.64.0.2", "CUSTOMER-A", "nat", models.SessionStateReleased))
	if dp.count() != 1 {
		t.Fatalf("release must not issue anything while the add is in flight, got %d calls", dp.count())
	}

	dp.complete(0, nil)
	if dp.count() != 2 || dp.call(1).isAdd {
		t.Fatalf("the committed mapping of a released session must be torn down, got %d calls", dp.count())
	}
	dp.complete(1, nil)

	if sid, _ := c.programmed("100.64.0.2", 100); sid != "" {
		t.Fatalf("nothing may remain programmed, got %q", sid)
	}
	if got := blocksHeld(c, "100.64.0.2", 100); got != 0 {
		t.Fatalf("block must be freed, got %d", got)
	}
	if keys := c.opdbKeys(t); len(keys) != 0 {
		t.Fatalf("opdb must be empty, got %v", keys)
	}
}

func TestLifecycle_OldReleaseDuringNewSessionsInFlightAdd(t *testing.T) {
	dp := &parkedDP{}
	c := newVRFComponent(t, dp)

	c.handleSessionActivate(ipoeEvent("a1", "100.64.0.2", "CUSTOMER-A", "nat", models.SessionStateActive))
	dp.complete(0, nil)
	ev := ipoeEvent("a2", "100.64.0.2", "CUSTOMER-A", "nat", models.SessionStateActive)
	ev.Session.(*models.IPoESession).IfIndex = 9
	c.handleSessionActivate(ev)
	c.handleSessionRelease(ipoeEvent("a1", "100.64.0.2", "CUSTOMER-A", "nat", models.SessionStateReleased))
	dp.complete(1, nil)

	if dp.count() != 2 {
		t.Fatalf("a1's release must not delete the block a2 is taking over, got %d calls", dp.count())
	}
	if sid, _ := c.programmed("100.64.0.2", 100); sid != "a2" {
		t.Fatalf("a2 must hold the key, got %q", sid)
	}
	if keys := c.opdbKeys(t); len(keys) != 1 || keys[0] != "a2" {
		t.Fatalf("opdb must record a2 only, got %v", keys)
	}
}

func TestLifecycle_DeleteFailureKeepsBlockUntilNextHolder(t *testing.T) {
	dp := &parkedDP{}
	c := newVRFComponent(t, dp)

	c.handleSessionActivate(ipoeEvent("a1", "100.64.0.2", "CUSTOMER-A", "nat", models.SessionStateActive))
	dp.complete(0, nil)
	c.handleSessionRelease(ipoeEvent("a1", "100.64.0.2", "CUSTOMER-A", "nat", models.SessionStateReleased))
	dp.complete(1, errors.New("reply timeout"))

	if got := blocksHeld(c, "100.64.0.2", 100); got != 1 {
		t.Fatalf("a failed delete must not free the block, got %d held", got)
	}
	if keys := c.opdbKeys(t); len(keys) != 1 || keys[0] != "a1" {
		t.Fatalf("a failed delete must keep the record, got %v", keys)
	}
	if sid, _ := c.programmed("100.64.0.2", 100); sid != "" {
		t.Fatalf("nothing is known programmed after a failed delete, got %q", sid)
	}

	c.handleSessionActivate(ipoeEvent("a2", "100.64.0.2", "CUSTOMER-A", "nat", models.SessionStateActive))
	if dp.count() != 3 || !dp.call(2).isAdd || dp.call(2).portStart != dp.call(0).portStart {
		t.Fatalf("the next holder must reuse and refresh the kept block, got %+v", dp.calls)
	}
	dp.complete(2, nil)

	if sid, _ := c.programmed("100.64.0.2", 100); sid != "a2" {
		t.Fatalf("a2 must hold the key, got %q", sid)
	}
	if keys := c.opdbKeys(t); len(keys) != 1 || keys[0] != "a2" {
		t.Fatalf("the takeover must re-key the record to a2, got %v", keys)
	}
}

func TestLifecycle_AddFailureReleasesOnlyAFreshBlock(t *testing.T) {
	dp := &parkedDP{}
	c := newVRFComponent(t, dp)

	c.handleSessionActivate(ipoeEvent("a1", "100.64.0.2", "CUSTOMER-A", "nat", models.SessionStateActive))
	dp.complete(0, errors.New("retval=-1"))

	if got := blocksHeld(c, "100.64.0.2", 100); got != 0 {
		t.Fatalf("a fresh block must be released when its add fails, got %d held", got)
	}
	c.subMu.Lock()
	entries := len(c.subs)
	c.subMu.Unlock()
	if entries != 0 {
		t.Fatalf("a key with nothing wanted, programmed or held must be dropped, got %d entries", entries)
	}

	// A block kept from a failed delete is not this session's to free.
	c.handleSessionActivate(ipoeEvent("a1", "100.64.0.2", "CUSTOMER-A", "nat", models.SessionStateActive))
	dp.complete(1, nil)
	c.handleSessionRelease(ipoeEvent("a1", "100.64.0.2", "CUSTOMER-A", "nat", models.SessionStateReleased))
	dp.complete(2, errors.New("reply timeout"))
	c.handleSessionActivate(ipoeEvent("a2", "100.64.0.2", "CUSTOMER-A", "nat", models.SessionStateActive))
	dp.complete(3, errors.New("retval=-1"))

	if got := blocksHeld(c, "100.64.0.2", 100); got != 1 {
		t.Fatalf("a reused block must survive a failed add, got %d held", got)
	}
}

func preserved(t *testing.T, c *Component, sessionID, ip string, vrf uint32) *models.CGNATMapping {
	t.Helper()
	mapping := &models.CGNATMapping{
		SessionID:      sessionID,
		PoolName:       "p1",
		PoolID:         1,
		InsideIP:       net.ParseIP(ip).To4(),
		InsideVRFID:    vrf,
		OutsideIP:      net.ParseIP("203.0.113.1").To4(),
		PortBlockStart: 1088,
		PortBlockEnd:   1151,
		SwIfIndex:      3,
	}
	if err := c.pools.RestoreMappingIfAbsent(mapping); err != nil {
		t.Fatalf("restore block: %v", err)
	}
	c.reverse.Add(mapping)
	c.preservedMapping(mapping)
	putMapping(t, c.opdb.(*fakeOpDB), mapping)
	return mapping
}

func TestLifecycle_PreservedBlock_RestoredEventProgramsIt(t *testing.T) {
	dp := &parkedDP{inline: true}
	c := newVRFComponent(t, dp)
	mapping := preserved(t, c, "s1", "100.64.0.2", 100)

	c.handleSessionActivate(ipoeEvent("s1", "100.64.0.2", "CUSTOMER-A", "nat", models.SessionStateActive))

	if dp.count() != 1 || !dp.call(0).isAdd || dp.call(0).portStart != mapping.PortBlockStart || dp.call(0).swIfIndex != 7 {
		t.Fatalf("the preserved block must be programmed for the restored session's interface, got %+v", dp.calls)
	}
	if sid, _ := c.programmed("100.64.0.2", 100); sid != "s1" {
		t.Fatalf("s1 must hold the key, got %q", sid)
	}
}

func TestLifecycle_PreservedBlock_ReleaseFreesIt(t *testing.T) {
	dp := &parkedDP{inline: true}
	c := newVRFComponent(t, dp)
	mapping := preserved(t, c, "s1", "100.64.0.2", 100)

	c.handleSessionRelease(ipoeEvent("s1", "100.64.0.2", "CUSTOMER-A", "nat", models.SessionStateReleased))

	if dp.count() != 1 || dp.call(0).isAdd || dp.call(0).portStart != mapping.PortBlockStart {
		t.Fatalf("the preserved block must be deleted from the dataplane, got %+v", dp.calls)
	}
	if got := blocksHeld(c, "100.64.0.2", 100); got != 0 {
		t.Fatalf("the preserved block must be freed, got %d held", got)
	}
	if keys := c.opdbKeys(t); len(keys) != 0 {
		t.Fatalf("the preserved record must be deleted, got %v", keys)
	}
	if c.reverse.Lookup(mapping.OutsideIP, mapping.PortBlockStart) != nil {
		t.Fatalf("reverse index must forget the freed block")
	}
	c.subMu.Lock()
	entries := len(c.subs)
	c.subMu.Unlock()
	if entries != 0 {
		t.Fatalf("no writer state may remain, got %d entries", entries)
	}
}

func TestLifecycle_PreservedBlock_OtherSessionsReleaseLeavesIt(t *testing.T) {
	dp := &parkedDP{inline: true}
	c := newVRFComponent(t, dp)
	preserved(t, c, "s1", "100.64.0.2", 100)

	c.handleSessionRelease(ipoeEvent("s2", "100.64.0.2", "CUSTOMER-A", "nat", models.SessionStateReleased))

	if dp.count() != 0 {
		t.Fatalf("a release by a session that does not own the block must do nothing, got %+v", dp.calls)
	}
	if got := blocksHeld(c, "100.64.0.2", 100); got != 1 {
		t.Fatalf("the preserved block must stay, got %d held", got)
	}
}

func TestLifecycle_BypassFollowsTheHolder(t *testing.T) {
	dp := &parkedDP{}
	c := newVRFComponent(t, dp)

	c.handleSessionActivate(ipoeEvent("a1", "100.64.0.2", "CUSTOMER-A", "bypass", models.SessionStateActive))
	c.handleSessionActivate(ipoeEvent("a2", "100.64.0.2", "CUSTOMER-A", "bypass", models.SessionStateActive))
	c.handleSessionRelease(ipoeEvent("a1", "100.64.0.2", "CUSTOMER-A", "bypass", models.SessionStateReleased))

	if len(dp.bypasses) != 1 || !dp.bypasses[0].isAdd {
		t.Fatalf("a1's release must not remove the bypass a2 holds, got %+v", dp.bypasses)
	}
	if !c.bypass.IsBypassed(net.ParseIP("100.64.0.2"), 100) {
		t.Fatalf("bypass must still be recorded")
	}

	c.handleSessionRelease(ipoeEvent("a2", "100.64.0.2", "CUSTOMER-A", "bypass", models.SessionStateReleased))

	if len(dp.bypasses) != 2 || dp.bypasses[1].isAdd {
		t.Fatalf("the holder's release must remove the bypass, got %+v", dp.bypasses)
	}
	if c.bypass.IsBypassed(net.ParseIP("100.64.0.2"), 100) {
		t.Fatalf("bypass still recorded after the holder's release")
	}
}

func TestLifecycle_ClassChangeTearsDownBeforeSetup(t *testing.T) {
	dp := &parkedDP{inline: true}
	c := newVRFComponent(t, dp)

	c.handleSessionActivate(ipoeEvent("a1", "100.64.0.2", "CUSTOMER-A", "nat", models.SessionStateActive))
	c.handleSessionActivate(ipoeEvent("a1", "100.64.0.2", "CUSTOMER-A", "bypass", models.SessionStateActive))

	if dp.count() != 2 || dp.call(1).isAdd {
		t.Fatalf("the PBA mapping must be deleted before the bypass is installed, got %+v", dp.calls)
	}
	if len(dp.bypasses) != 1 || !dp.bypasses[0].isAdd {
		t.Fatalf("the bypass must be installed after the teardown, got %+v", dp.bypasses)
	}
	if got := blocksHeld(c, "100.64.0.2", 100); got != 0 {
		t.Fatalf("no block may be held by a bypass session, got %d", got)
	}
	if sid, pool := c.programmed("100.64.0.2", 100); sid != "a1" || pool != "" {
		t.Fatalf("a1 must hold the key as bypass, got %q %q", sid, pool)
	}
}

func TestLifecycle_ReactivateSameSessionIsNoOp(t *testing.T) {
	dp := &parkedDP{inline: true}
	c := newVRFComponent(t, dp)

	c.handleSessionActivate(ipoeEvent("a1", "100.64.0.2", "CUSTOMER-A", "nat", models.SessionStateActive))
	c.handleSessionActivate(ipoeEvent("a1", "100.64.0.2", "CUSTOMER-A", "nat", models.SessionStateActive))

	if dp.count() != 1 {
		t.Fatalf("a repeated activate for the programmed session must not touch the dataplane, got %d calls", dp.count())
	}
}

// TestLifecycle_OneWriterPerKeyUnderChurn fires activates and releases
// for three sessions on one key from many goroutines while callbacks
// complete on their own goroutine in arbitrary order. The dataplane must
// never see two calls in flight for the key, and once quiet the
// allocator, opdb and writer must agree on one holder or none.
func TestLifecycle_OneWriterPerKeyUnderChurn(t *testing.T) {
	dp := &parkedDP{}
	c := newVRFComponent(t, dp)

	stop := make(chan struct{})
	var completer sync.WaitGroup
	completer.Add(1)
	go func() {
		defer completer.Done()
		rng := rand.New(rand.NewSource(1))
		for {
			dp.mu.Lock()
			var pending []asyncCall
			for i, done := range dp.completed {
				if !done {
					pending = append(pending, dp.calls[i])
				}
			}
			dp.mu.Unlock()
			if len(pending) == 0 {
				select {
				case <-stop:
					return
				default:
					time.Sleep(50 * time.Microsecond)
					continue
				}
			}
			call := pending[rng.Intn(len(pending))]
			if rng.Intn(10) == 0 {
				call.done(errors.New("injected"))
			} else {
				call.done(nil)
			}
		}
	}()

	sessions := []string{"a1", "a2", "a3"}
	var producers sync.WaitGroup
	for g := 0; g < 8; g++ {
		producers.Add(1)
		go func(seed int64) {
			defer producers.Done()
			rng := rand.New(rand.NewSource(seed))
			for i := 0; i < 200; i++ {
				sid := sessions[rng.Intn(len(sessions))]
				if rng.Intn(2) == 0 {
					ev := ipoeEvent(sid, "100.64.0.2", "CUSTOMER-A", "nat", models.SessionStateActive)
					ev.Session.(*models.IPoESession).IfIndex = uint32(7 + rng.Intn(3))
					c.handleSessionActivate(ev)
				} else {
					c.handleSessionRelease(ipoeEvent(sid, "100.64.0.2", "CUSTOMER-A", "nat", models.SessionStateReleased))
				}
			}
		}(int64(g))
	}
	producers.Wait()

	deadline := time.Now().Add(5 * time.Second)
	for {
		dp.mu.Lock()
		quiet := dp.inflight == 0
		dp.mu.Unlock()
		if quiet {
			c.subMu.Lock()
			st := c.subs[makeSubscriberKey(100, net.ParseIP("100.64.0.2"))]
			quiet = st == nil || !st.busy
			c.subMu.Unlock()
		}
		if quiet || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	close(stop)
	completer.Wait()

	if dp.maxInflight != 1 {
		t.Fatalf("one dataplane call in flight per key, saw %d", dp.maxInflight)
	}
	sid, _ := c.programmed("100.64.0.2", 100)
	held := blocksHeld(c, "100.64.0.2", 100)
	keys := c.opdbKeys(t)
	switch {
	case sid != "":
		if held != 1 {
			t.Fatalf("holder %s but %d blocks held", sid, held)
		}
		if len(keys) != 1 || keys[0] != sid {
			t.Fatalf("holder %s but opdb has %v", sid, keys)
		}
	default:
		// A failed delete keeps its block and record on purpose; either
		// both remain or neither does.
		if (held == 0) != (len(keys) == 0) {
			t.Fatalf("no holder but %d blocks held and opdb has %v", held, keys)
		}
	}
}
