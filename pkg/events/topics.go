// Copyright 2025 Veesix Networks Ltd
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
)
