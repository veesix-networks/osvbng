// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package vpp

import (
	"fmt"
	"sync"

	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/acl"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface_types"
)

// aclRegistry tracks the name -> VPP ACL index mapping. ACL config-schema
// work that populates this registry is tracked separately; until that lands,
// ApplyIngressACL/ApplyEgressACL with a non-empty name resolves to "unknown
// ACL" and the southbound logs a structured warning so operators can see the
// gap. The per-interface list-set call is still issued on Remove and on
// Apply-with-empty-name so the dataplane state always tracks the caller's
// intent for the names that ARE wired.
type aclRegistry struct {
	mu        sync.RWMutex
	nameToIdx map[string]uint32
}

func newACLRegistry() *aclRegistry {
	return &aclRegistry{nameToIdx: make(map[string]uint32)}
}

func (r *aclRegistry) lookup(name string) (uint32, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	idx, ok := r.nameToIdx[name]
	return idx, ok
}

// RegisterACL records the name -> index mapping returned by acl_add_replace.
// Future ACL handler work in pkg/handlers/conf/acl/ will call this after
// programming each operator-configured ACL into VPP.
func (v *VPP) RegisterACL(name string, index uint32) {
	v.aclReg.mu.Lock()
	defer v.aclReg.mu.Unlock()
	v.aclReg.nameToIdx[name] = index
}

// UnregisterACL removes the name -> index mapping. Called by the ACL handler
// when an operator deletes an ACL from configuration.
func (v *VPP) UnregisterACL(name string) {
	v.aclReg.mu.Lock()
	defer v.aclReg.mu.Unlock()
	delete(v.aclReg.nameToIdx, name)
}

// ApplyIngressACL replaces the inbound ACL list on swIfIndex with [aclName].
// If aclName is empty, the call clears the inbound list (equivalent to
// RemoveIngressACL). Unknown names are reported as an error so callers can
// decide whether to abort session bring-up.
func (v *VPP) ApplyIngressACL(swIfIndex uint32, aclName string) error {
	return v.setACLList(swIfIndex, aclName, "")
}

// ApplyEgressACL is the egress counterpart of ApplyIngressACL.
func (v *VPP) ApplyEgressACL(swIfIndex uint32, aclName string) error {
	return v.setACLList(swIfIndex, "", aclName)
}

// RemoveIngressACL clears the inbound ACL list on swIfIndex.
func (v *VPP) RemoveIngressACL(swIfIndex uint32) error {
	return v.setACLList(swIfIndex, "", "")
}

// RemoveEgressACL clears the outbound ACL list on swIfIndex.
func (v *VPP) RemoveEgressACL(swIfIndex uint32) error {
	return v.setACLList(swIfIndex, "", "")
}

func (v *VPP) setACLList(swIfIndex uint32, ingress, egress string) error {
	var acls []uint32
	var nInput uint8

	if ingress != "" {
		idx, ok := v.aclReg.lookup(ingress)
		if !ok {
			v.logger.Warn("ACL name not registered; ingress binding skipped",
				"acl_name", ingress, "sw_if_index", swIfIndex)
			return fmt.Errorf("unknown ingress ACL %q", ingress)
		}
		acls = append(acls, idx)
		nInput++
	}
	if egress != "" {
		idx, ok := v.aclReg.lookup(egress)
		if !ok {
			v.logger.Warn("ACL name not registered; egress binding skipped",
				"acl_name", egress, "sw_if_index", swIfIndex)
			return fmt.Errorf("unknown egress ACL %q", egress)
		}
		acls = append(acls, idx)
	}

	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &acl.ACLInterfaceSetACLList{
		SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
		NInput:    nInput,
		Acls:      acls,
	}
	reply := &acl.ACLInterfaceSetACLListReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("acl_interface_set_acl_list: %w", err)
	}
	if reply.Retval != 0 {
		return fmt.Errorf("acl_interface_set_acl_list retval=%d", reply.Retval)
	}
	return nil
}
