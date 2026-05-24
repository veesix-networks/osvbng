// Copyright 2025 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package events

const (
	TopicSessionLifecycle   = "osvbng:events:session:lifecycle"
	TopicSessionProgrammed  = "osvbng:events:session:programmed"
	TopicAAARequest        = "osvbng:events:aaa:request"
	TopicAAAResponse       = "osvbng:events:aaa:response"
	TopicAAAResponseIPoE   = "osvbng:events:aaa:response:ipoe"
	TopicAAAResponsePPPoE  = "osvbng:events:aaa:response:pppoe"
	TopicEgress            = "osvbng:events:egress"
	TopicHAStateChange     = "osvbng:events:ha:state_change"
	TopicInterfaceState    = "osvbng:events:interface:state"
	TopicCGNATMapping             = "osvbng:events:cgnat:mapping"
	TopicSubscriberMutation       = "osvbng:events:subscriber:mutation"
	TopicSubscriberMutationResult = "osvbng:events:subscriber:mutation:result"
	TopicSubscriberTerminate      = "osvbng:events:subscriber:terminate"

	// L2TPv2 topics — see components/l2tp/60-l2tpv2/IMPLEMENTATION_SPEC.md
	// §"Shared-core performance considerations" (spec-finalize C4).
	TopicAAAResponseL2TP = "osvbng:events:aaa:response:l2tp"
	TopicL2TPLACDecision = "osvbng:events:l2tp:lac:decision"

	// TopicComponentReady fires when a component transitions out of its
	// recovery window into StateReady. Carries a ComponentReadyEvent.
	// Consumers that depend on a specific component's recovery completing
	// (CGNAT waiting for IPoE/PPPoE TopicSessionRestored flush, etc.)
	// subscribe to this rather than busy-polling the Base.ReadyState().
	TopicComponentReady = "osvbng:events:component:ready"
)
