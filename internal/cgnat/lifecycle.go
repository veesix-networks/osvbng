// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package cgnat

import (
	"context"
	"encoding/json"
	"net"

	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/opdb"
)

// Session lifecycle runs through one writer per subscriber key, the
// (inside VRF, inside IP) identity of ADR 0006. The event bus delivers
// every handler on its own goroutine and a subscriber's activate and
// release arrive on different topics, so nothing upstream orders them:
// with a sticky DHCP lease the new session's activate and the old
// session's release meet on the same key, and whichever dataplane call
// lands last wins. The writer removes the interleaving instead of
// tolerating it. Events only record which session should hold the key
// (want); converge issues at most one dataplane call per key at a time
// and re-evaluates when it completes. There is no queue: a burst of
// events for one key collapses into its latest want, so the writer is
// bounded by construction and spawns nothing.
//
// Ownership follows from the same state. A release only clears want
// when the releasing session is the one that holds it, so the old
// session's release after the new session took the key over is a
// no-op, and the block goes back to the allocator only from the
// delete callback on success.

type sessionRef struct {
	sessionID string
	swIfIndex uint32
	srgName   string
	class     classification
}

// subscriberState is one key's writer. want and have are compared by
// value in next(); block mirrors the allocator's entry for the key and
// can outlive have when the restore loop preserved a block for a session
// it could not find (see restoreFromOpDB).
type subscriberState struct {
	want  *sessionRef
	have  *sessionRef
	block *models.CGNATMapping
	busy  bool
}

type lifecycleStep int

const (
	stepNone lifecycleStep = iota
	stepSetup
	stepTeardown
	stepRefresh
	stepAdopt
)

// next picks the single step that moves have toward want. Refresh is
// the PBA path for a key changing hands or interface: the block stays,
// the plugin's add refreshes the stored sw_if_index. Adopt records a
// change that needs no dataplane call because bypass is keyed by the
// address alone.
func (st *subscriberState) next() (lifecycleStep, *sessionRef) {
	w, h := st.want, st.have
	switch {
	case h == nil && w == nil:
		return stepNone, nil
	case h == nil:
		return stepSetup, w
	case w == nil:
		return stepTeardown, h
	case *w == *h:
		return stepNone, nil
	case w.class != h.class:
		return stepTeardown, h
	}
	switch w.class.kind {
	case classPBA:
		return stepRefresh, w
	case classBypass:
		return stepAdopt, w
	case classDeterministic:
		if w.swIfIndex != h.swIfIndex {
			return stepTeardown, h
		}
		return stepAdopt, w
	}
	return stepNone, nil
}

type sessionFields struct {
	insideIP     net.IP
	vrfName      string
	swIfIndex    uint32
	serviceGroup string
	srgName      string
}

func lifecycleFields(data *events.SessionLifecycleEvent) (sessionFields, bool) {
	var f sessionFields
	switch data.AccessType {
	case models.AccessTypeIPoE:
		sess, ok := data.Session.(*models.IPoESession)
		if !ok {
			return f, false
		}
		f = sessionFields{sess.IPv4Address, sess.VRF, sess.IfIndex, sess.ServiceGroup, sess.SRGName}
	case models.AccessTypePPPoE:
		sess, ok := data.Session.(*models.PPPSession)
		if !ok {
			return f, false
		}
		f = sessionFields{sess.IPv4Address, sess.VRF, sess.IfIndex, sess.ServiceGroup, sess.SRGName}
	default:
		return f, false
	}
	if f.insideIP == nil || f.insideIP.To4() == nil {
		return f, false
	}
	return f, true
}

func (c *Component) handleSessionActivate(data *events.SessionLifecycleEvent) {
	f, ok := lifecycleFields(data)
	if !ok {
		return
	}
	vrfID, err := c.sessionVRFID(f.vrfName)
	if err != nil {
		c.logger.Error("CGNAT activation refused: session VRF does not resolve",
			"session", data.SessionID, "vrf", f.vrfName, "error", err)
		return
	}
	c.activate(makeSubscriberKey(vrfID, f.insideIP), data.SessionID, f)
}

// activate is the entry for live events and for the restore scan. A
// session whose classification is none holds nothing, so it is treated
// as its own release.
func (c *Component) activate(key subscriberKey, sessionID string, f sessionFields) {
	cls := c.classifySession(f.serviceGroup, f.insideIP, f.vrfName)
	if cls.kind == classNone {
		c.clearWant(key, sessionID)
	} else {
		c.setWant(key, &sessionRef{sessionID: sessionID, swIfIndex: f.swIfIndex, srgName: f.srgName, class: cls})
	}
	c.converge(key)
}

func (c *Component) handleSessionRelease(data *events.SessionLifecycleEvent) {
	f, ok := lifecycleFields(data)
	if !ok {
		return
	}
	vrfID, err := c.sessionVRFID(f.vrfName)
	if err != nil {
		c.logger.Error("CGNAT release: session VRF does not resolve, mapping not released",
			"session", data.SessionID, "inside_ip", f.insideIP, "vrf", f.vrfName, "error", err)
		return
	}
	key := makeSubscriberKey(vrfID, f.insideIP)
	c.clearWant(key, data.SessionID)
	c.converge(key)
}

func (c *Component) setWant(key subscriberKey, ref *sessionRef) {
	c.subMu.Lock()
	defer c.subMu.Unlock()
	st := c.subs[key]
	if st == nil {
		st = &subscriberState{}
		c.subs[key] = st
	}
	st.want = ref
}

// clearWant is the ownership check: only the session that holds the
// key releases it. A block the restore loop preserved for this session
// with nothing programmed is handed to the normal teardown, because
// after a daemon-only restart VPP may still hold that mapping and the
// plugin's delete of a missing entry succeeds.
func (c *Component) clearWant(key subscriberKey, sessionID string) {
	c.subMu.Lock()
	defer c.subMu.Unlock()
	st := c.subs[key]
	if st == nil {
		return
	}
	if st.want != nil && st.want.sessionID == sessionID {
		st.want = nil
	}
	if st.want == nil && st.have == nil && st.block != nil && st.block.SessionID == sessionID {
		st.have = &sessionRef{
			sessionID: sessionID,
			swIfIndex: st.block.SwIfIndex,
			class:     classification{kind: classPBA, poolName: st.block.PoolName},
		}
	}
}

// converge takes one step for the key unless one is in flight. Every
// step ends in finish, which re-enters converge, so a key that needs
// several steps (tear down one class, set up another) walks them in
// order on whichever goroutine completed the previous one.
func (c *Component) converge(key subscriberKey) {
	c.subMu.Lock()
	st := c.subs[key]
	if st == nil || st.busy {
		c.subMu.Unlock()
		return
	}
	step, ref := st.next()
	if step == stepNone {
		if st.want == nil && st.have == nil && st.block == nil {
			delete(c.subs, key)
		}
		c.subMu.Unlock()
		return
	}
	st.busy = true
	block := st.block
	c.subMu.Unlock()

	switch step {
	case stepSetup, stepRefresh:
		c.setup(key, ref, block)
	case stepTeardown:
		c.teardown(key, ref, block)
	case stepAdopt:
		c.finish(key, func(st *subscriberState) { st.have = ref })
	}
}

// finish applies a step's outcome and hands the key back. apply runs
// under the lock and must only touch the state; every side effect
// (dataplane, allocator, opdb, bus) happens before.
func (c *Component) finish(key subscriberKey, apply func(st *subscriberState)) {
	c.subMu.Lock()
	if st := c.subs[key]; st != nil {
		apply(st)
		st.busy = false
	}
	c.subMu.Unlock()
	c.converge(key)
}

// failWant drops the desire the failed step was serving. want is
// compared by pointer so a newer desire set while the step ran is kept
// and converge retries against it.
func failWant(ref *sessionRef) func(st *subscriberState) {
	return func(st *subscriberState) {
		if st.want == ref {
			st.want = nil
		}
	}
}

func (c *Component) setup(key subscriberKey, ref *sessionRef, prev *models.CGNATMapping) {
	switch ref.class.kind {
	case classPBA:
		c.addMapping(key, ref, prev)
	case classBypass:
		if err := c.programBypass(key, true); err != nil {
			c.logger.Error("bypass programming failed",
				"session", ref.sessionID, "ip", keyIP(key), "vrf_id", key.InsideVRF, "error", err)
			c.finish(key, failWant(ref))
			return
		}
		c.finish(key, func(st *subscriberState) { st.have = ref })
	case classDeterministic:
		if err := c.programDeterministic(ref.class.poolName, ref.swIfIndex, true); err != nil {
			c.logger.Error("enable CGNAT on session failed",
				"session", ref.sessionID, "pool", ref.class.poolName, "sw_if", ref.swIfIndex, "error", err)
			c.finish(key, failWant(ref))
			return
		}
		c.finish(key, func(st *subscriberState) { st.have = ref })
	default:
		c.finish(key, failWant(ref))
	}
}

func (c *Component) teardown(key subscriberKey, ref *sessionRef, prev *models.CGNATMapping) {
	switch ref.class.kind {
	case classPBA:
		c.deleteMapping(key, ref, prev)
	case classBypass:
		if err := c.programBypass(key, false); err != nil {
			c.logger.Error("bypass removal failed",
				"session", ref.sessionID, "ip", keyIP(key), "vrf_id", key.InsideVRF, "error", err)
		}
		prefix := net.IPNet{IP: keyIP(key), Mask: net.CIDRMask(32, 32)}
		c.bypass.RemovePrefix(&prefix, key.InsideVRF)
		c.finish(key, func(st *subscriberState) { st.have = nil })
	case classDeterministic:
		if err := c.programDeterministic(ref.class.poolName, ref.swIfIndex, false); err != nil {
			c.logger.Debug("Disable CGNAT on session", "sw_if", ref.swIfIndex, "error", err)
		}
		c.finish(key, func(st *subscriberState) { st.have = nil })
	default:
		c.finish(key, func(st *subscriberState) { st.have = nil })
	}
}

// addMapping programs the key's block for ref. With prev nil the block
// comes from the HA-synced record for the session if a peer sent one,
// else from the allocator; with prev set the block is kept and the add
// refreshes the plugin's sw_if_index for the new holder. A reused block
// is programmed regardless: committing it without a dataplane call left
// the new subscriber forwarding untranslated (audit B1).
func (c *Component) addMapping(key subscriberKey, ref *sessionRef, prev *models.CGNATMapping) {
	poolName := ref.class.poolName
	insideIP := keyIP(key)
	vrfID := key.InsideVRF
	poolID, ok := c.poolIDMap[poolName]
	if !ok {
		c.logger.Error("CGNAT activation refused: pool not registered", "session", ref.sessionID, "pool", poolName)
		c.finish(key, failWant(ref))
		return
	}

	var (
		mapping  *models.CGNATMapping
		isNew    bool
		fromPeer bool
	)
	if prev == nil {
		if synced := c.loadSyncedMapping(ref.sessionID); synced != nil {
			synced.InsideVRFID = vrfID
			synced.SwIfIndex = ref.swIfIndex
			if err := c.pools.RestoreMappingIfAbsent(synced); err != nil {
				c.logger.Warn("Failed to restore synced CGNAT mapping", "session", ref.sessionID, "error", err)
			} else {
				mapping, isNew, fromPeer = synced, true, true
			}
		}
	}
	if mapping == nil {
		var err error
		mapping, isNew, err = c.pools.GetOrAllocate(poolName, insideIP, vrfID, ref.swIfIndex)
		if err != nil {
			c.storm.Error(poolName, "CGNAT block allocation failed",
				"pool", poolName, "session", ref.sessionID, "ip", insideIP, "vrf_id", vrfID, "error", err)
			c.finish(key, failWant(ref))
			return
		}
	}
	mapping.SessionID = ref.sessionID
	mapping.SwIfIndex = ref.swIfIndex

	c.dataplane.CGNATAddDelSubscriberMappingAsync(poolID, ref.swIfIndex, insideIP,
		vrfID, mapping.OutsideIP, mapping.PortBlockStart, mapping.PortBlockEnd,
		true, true, func(err error) {
			if err != nil {
				c.storm.Error(poolName, "subscriber mapping failed",
					"pool", poolName, "session", ref.sessionID, "inside", insideIP, "vrf_id", vrfID, "error", err)
				if isNew {
					if rerr := c.pools.ReleaseBlocks(poolName, insideIP, vrfID); rerr != nil {
						c.logger.Error("CGNAT rollback: release blocks failed",
							"session", ref.sessionID, "inside", insideIP, "vrf_id", vrfID, "error", rerr)
					}
				}
				c.finish(key, func(st *subscriberState) {
					failWant(ref)(st)
					if isNew {
						st.block = nil
					}
				})
				return
			}

			if prev != nil {
				c.reverse.Remove(prev.OutsideIP, prev.PortBlockStart)
				if prev.SessionID != ref.sessionID {
					c.forgetMapping(opdbNamespace, prev.SessionID)
					c.publishMappingEvent(ref.srgName, prev, false)
				}
			}
			c.reverse.Add(mapping)
			c.persistMapping(context.Background(), mapping)
			if fromPeer {
				c.forgetMapping(opdb.NamespaceHASyncedCGNAT, ref.sessionID)
			}
			if prev == nil || prev.SessionID != ref.sessionID {
				c.publishMappingEvent(ref.srgName, mapping, true)
			}
			c.finish(key, func(st *subscriberState) {
				st.have = ref
				st.block = mapping
			})

			c.logger.Debug("CGNAT PBA mapping programmed",
				"session", ref.sessionID,
				"inside", insideIP,
				"vrf_id", vrfID,
				"reused_block", !isNew,
				"from_peer", fromPeer,
				"outside", mapping.OutsideIP,
				"ports", mapping.PortBlockStart, mapping.PortBlockEnd,
				"pool", poolName)
		})
}

// deleteMapping frees the block only from the callback on success. On
// failure the block stays allocated and the opdb record stays, so the
// next activate for the key reuses and refreshes it and the next
// restart's restore sees it; nothing is freed that VPP may still hold.
func (c *Component) deleteMapping(key subscriberKey, ref *sessionRef, prev *models.CGNATMapping) {
	if prev == nil {
		c.finish(key, func(st *subscriberState) { st.have = nil })
		return
	}
	poolName := ref.class.poolName
	insideIP := keyIP(key)
	vrfID := key.InsideVRF
	poolID := c.poolIDMap[poolName]

	c.dataplane.CGNATAddDelSubscriberMappingAsync(poolID, ref.swIfIndex, insideIP,
		vrfID, prev.OutsideIP, prev.PortBlockStart, prev.PortBlockEnd,
		false, false, func(err error) {
			if err != nil {
				c.storm.Error(poolName, "remove mapping failed; block kept allocated",
					"pool", poolName, "session", ref.sessionID, "inside", insideIP, "vrf_id", vrfID, "error", err)
				c.finish(key, func(st *subscriberState) { st.have = nil })
				return
			}
			c.reverse.Remove(prev.OutsideIP, prev.PortBlockStart)
			if rerr := c.pools.ReleaseBlocks(poolName, insideIP, vrfID); rerr != nil {
				c.logger.Error("CGNAT release: release blocks failed",
					"session", ref.sessionID, "inside", insideIP, "vrf_id", vrfID, "error", rerr)
			}
			c.forgetMapping(opdbNamespace, prev.SessionID)
			c.publishMappingEvent(ref.srgName, prev, false)
			c.finish(key, func(st *subscriberState) {
				st.have = nil
				st.block = nil
			})
			c.logger.Debug("CGNAT mapping released", "session", ref.sessionID, "inside", insideIP, "vrf_id", vrfID)
		})
}

// persistMapping and forgetMapping keep opdb in step with the writer. A
// failed write is logged and the in-memory state stays authoritative
// (ADR 0006); the next restore sees whatever opdb last held.
func (c *Component) persistMapping(ctx context.Context, mapping *models.CGNATMapping) {
	if c.opdb == nil {
		return
	}
	data, err := json.Marshal(mapping)
	if err != nil {
		return
	}
	if err := c.opdb.Put(ctx, opdbNamespace, mapping.SessionID, data); err != nil {
		c.logger.Warn("CGNAT mapping checkpoint failed", "session", mapping.SessionID, "error", err)
	}
}

func (c *Component) forgetMapping(namespace, sessionID string) {
	if c.opdb == nil {
		return
	}
	if err := c.opdb.Delete(context.Background(), namespace, sessionID); err != nil {
		c.logger.Warn("CGNAT mapping checkpoint delete failed", "namespace", namespace, "session", sessionID, "error", err)
	}
}

func (c *Component) loadSyncedMapping(sessionID string) *models.CGNATMapping {
	if c.opdb == nil {
		return nil
	}
	var found []byte
	err := c.opdb.Load(context.Background(), opdb.NamespaceHASyncedCGNAT, func(key string, value []byte) error {
		if key == sessionID {
			found = append([]byte(nil), value...)
		}
		return nil
	})
	if err != nil {
		c.logger.Warn("CGNAT HA-synced mapping load failed", "session", sessionID, "error", err)
		return nil
	}
	if found == nil {
		return nil
	}
	var mapping models.CGNATMapping
	if err := json.Unmarshal(found, &mapping); err != nil {
		return nil
	}
	mapping.SessionID = sessionID
	return &mapping
}

func (c *Component) programBypass(key subscriberKey, isAdd bool) error {
	prefix := net.IPNet{IP: keyIP(key), Mask: net.CIDRMask(32, 32)}
	if err := c.dataplane.CGNATAddDelBypass(prefix, key.InsideVRF, isAdd); err != nil {
		return err
	}
	if isAdd {
		c.bypass.AddIP(prefix.IP, key.InsideVRF)
	}
	return nil
}

func (c *Component) programDeterministic(poolName string, swIfIndex uint32, isEnable bool) error {
	poolID := c.poolIDMap[poolName]
	return c.dataplane.CGNATEnableOnSession(poolID, swIfIndex, isEnable)
}

func keyIP(key subscriberKey) net.IP {
	return net.IP(append([]byte(nil), key.InsideIP[:]...))
}

// restoredMapping records a mapping the restore loop programmed through
// the bulk path as the key's programmed and wanted state.
func (c *Component) restoredMapping(mapping *models.CGNATMapping, srgName string) {
	ref := &sessionRef{
		sessionID: mapping.SessionID,
		swIfIndex: mapping.SwIfIndex,
		srgName:   srgName,
		class:     classification{kind: classPBA, poolName: mapping.PoolName},
	}
	key := makeSubscriberKey(mapping.InsideVRFID, mapping.InsideIP)
	c.subMu.Lock()
	defer c.subMu.Unlock()
	st := c.subs[key]
	if st == nil {
		st = &subscriberState{}
		c.subs[key] = st
	}
	st.have = ref
	st.want = ref
	st.block = mapping
}

// preservedMapping records a block the restore loop kept for a session
// it could not find in the subscriber cache. Nothing is programmed: the
// session's restored event programs it through setup, its release frees
// it through teardown (clearWant), and until one arrives the block is
// held so the subscriber keeps its outside identity.
func (c *Component) preservedMapping(mapping *models.CGNATMapping) {
	key := makeSubscriberKey(mapping.InsideVRFID, mapping.InsideIP)
	c.subMu.Lock()
	defer c.subMu.Unlock()
	st := c.subs[key]
	if st == nil {
		st = &subscriberState{}
		c.subs[key] = st
	}
	st.block = mapping
}

// holder reports the session that has the key programmed or wanted.
// Used by the restore scan to skip sessions already in hand.
func (c *Component) holder(key subscriberKey) (string, bool) {
	c.subMu.Lock()
	defer c.subMu.Unlock()
	st := c.subs[key]
	if st == nil {
		return "", false
	}
	if st.have != nil {
		return st.have.sessionID, true
	}
	if st.want != nil {
		return st.want.sessionID, true
	}
	return "", false
}
