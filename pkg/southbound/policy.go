// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package southbound

// Policy groups per-interface policy programming distinct from session-table
// management. Implementations must treat every method as idempotent: applying
// the same configuration twice is a no-op, and re-applying with different
// arguments replaces the existing programming in a single transition.
//
// The methods here are the integration surface used by pkg/svcgroup to drive
// the service-group bindings of a subscriber session (ACL, URPF). QoS and
// scheduler programming still lives on the Sessions interface for backwards
// compatibility; future work may consolidate them here.
type Policy interface {
	// ApplyIngressACL replaces the inbound ACL list on the interface with
	// the single ACL named aclName. An empty aclName is a no-op for
	// callers that have nothing to apply. Names that do not resolve in
	// the dataplane's ACL registry must be reported as an error so the
	// caller can decide whether to fail the session bring-up.
	ApplyIngressACL(swIfIndex uint32, aclName string) error

	// ApplyEgressACL is the egress counterpart of ApplyIngressACL.
	ApplyEgressACL(swIfIndex uint32, aclName string) error

	// RemoveIngressACL clears any inbound ACL programming on the
	// interface. Idempotent on an interface with no ACL set.
	RemoveIngressACL(swIfIndex uint32) error

	// RemoveEgressACL clears any outbound ACL programming on the
	// interface. Idempotent on an interface with no ACL set.
	RemoveEgressACL(swIfIndex uint32) error

	// EnableSourceVerify turns on uRPF on the interface in the chosen
	// mode (strict if strict==true, loose otherwise). Applies to both
	// IPv4 and IPv6 in the inbound direction.
	EnableSourceVerify(swIfIndex uint32, strict bool) error

	// DisableSourceVerify clears any uRPF programming on the interface
	// for both IPv4 and IPv6 in the inbound direction.
	DisableSourceVerify(swIfIndex uint32) error
}
