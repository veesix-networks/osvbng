// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package svcgroup

import (
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/config/qos"
	"github.com/veesix-networks/osvbng/pkg/logger"
)

// PolicyApplier is the narrow southbound surface that ApplyToSession /
// ReverseFromSession use. It is satisfied by southbound.Southbound but
// declared locally so tests can substitute a fake without pulling in the
// full dataplane interface, and so this package stays free of an import
// cycle with pkg/southbound.
type PolicyApplier interface {
	ApplyIngressACL(swIfIndex uint32, aclName string) error
	ApplyEgressACL(swIfIndex uint32, aclName string) error
	RemoveIngressACL(swIfIndex uint32) error
	RemoveEgressACL(swIfIndex uint32) error

	EnableSourceVerify(swIfIndex uint32, strict bool) error
	DisableSourceVerify(swIfIndex uint32) error

	ApplyQoS(swIfIndex uint32, ingress, egress *qos.Policy) error
	RemoveQoS(swIfIndex uint32) error
	ApplyScheduler(swIfIndex uint32, rateKbps uint32, cfg *qos.SchedulerConfig) error
	RemoveScheduler(swIfIndex uint32) error
}

// ApplyToSession programs every per-session policy binding implied by sg
// onto swIfIndex: uRPF, ingress ACL, egress ACL, ingress QoS / scheduler /
// egress QoS. Each underlying southbound call is idempotent — re-applying
// the same configuration is a no-op — so this is safe to invoke both at
// fresh post-auth bring-up AND during opdb restore, where the dataplane
// state may already match.
//
// qosPolicies is the runningConfig's qos.Policy registry; service-group
// references to QoS policy names are resolved against it.
//
// Policy programming proceeds in this order: uRPF, ACLs, then QoS. ACL
// resolution failures are surfaced as errors so the caller can decide
// whether to abort bring-up; QoS failures are logged but do not abort,
// matching the prior best-effort behaviour of the subscriber-component
// activateSession path this code lifted from.
func ApplyToSession(sb PolicyApplier, swIfIndex uint32, sg ServiceGroup, qosPolicies map[string]*qos.Policy) error {
	log := logger.Get(logger.SvcGroup)

	switch sg.URPF {
	case "", "off":
	case "strict":
		if err := sb.EnableSourceVerify(swIfIndex, true); err != nil {
			return fmt.Errorf("apply urpf strict: %w", err)
		}
	case "loose":
		if err := sb.EnableSourceVerify(swIfIndex, false); err != nil {
			return fmt.Errorf("apply urpf loose: %w", err)
		}
	default:
		log.Warn("Unknown uRPF mode; skipping",
			"urpf", sg.URPF, "sw_if_index", swIfIndex, "service_group", sg.Name)
	}

	if sg.ACLIngress != "" {
		if err := sb.ApplyIngressACL(swIfIndex, sg.ACLIngress); err != nil {
			return fmt.Errorf("apply ingress acl %q: %w", sg.ACLIngress, err)
		}
	}
	if sg.ACLEgress != "" {
		if err := sb.ApplyEgressACL(swIfIndex, sg.ACLEgress); err != nil {
			return fmt.Errorf("apply egress acl %q: %w", sg.ACLEgress, err)
		}
	}

	var ingress, egress *qos.Policy
	if sg.QoSIngress != "" {
		ingress = qosPolicies[sg.QoSIngress]
	}
	if sg.QoSEgress != "" {
		egress = qosPolicies[sg.QoSEgress]
	}
	if ingress == nil && egress == nil {
		return nil
	}

	if egress != nil && egress.Scheduler != nil {
		downloadRate := egress.CIR
		if sg.DownloadRate > 0 {
			downloadRate = uint32(sg.DownloadRate)
		}
		if downloadRate > 0 {
			if err := sb.ApplyScheduler(swIfIndex, downloadRate, egress.Scheduler); err != nil {
				log.Warn("Failed to apply scheduler",
					"error", err, "sw_if_index", swIfIndex, "service_group", sg.Name)
			} else {
				log.Debug("Applied CAKE scheduler",
					"sw_if_index", swIfIndex,
					"service_group", sg.Name,
					"rate_kbps", downloadRate,
					"tin_mode", egress.Scheduler.TinMode)
			}
		}
		if ingress != nil {
			if err := sb.ApplyQoS(swIfIndex, ingress, nil); err != nil {
				log.Warn("Failed to apply ingress policer",
					"error", err, "sw_if_index", swIfIndex, "service_group", sg.Name)
			}
		}
		return nil
	}

	if err := sb.ApplyQoS(swIfIndex, ingress, egress); err != nil {
		log.Warn("Failed to apply QoS",
			"error", err, "sw_if_index", swIfIndex, "service_group", sg.Name)
		return nil
	}
	log.Debug("Applied QoS",
		"sw_if_index", swIfIndex,
		"service_group", sg.Name,
		"ingress_policy", sg.QoSIngress,
		"egress_policy", sg.QoSEgress)
	return nil
}

// ReverseFromSession unwinds every binding ApplyToSession installed for sg
// in inverse order: QoS / scheduler, then ACLs, then uRPF. Best-effort:
// individual step failures are logged but do not abort the rest of the
// teardown, and "already removed" is treated as success.
func ReverseFromSession(sb PolicyApplier, swIfIndex uint32, sg ServiceGroup) {
	log := logger.Get(logger.SvcGroup)

	if err := sb.RemoveScheduler(swIfIndex); err != nil {
		log.Debug("RemoveScheduler error during teardown",
			"error", err, "sw_if_index", swIfIndex)
	}
	if err := sb.RemoveQoS(swIfIndex); err != nil {
		log.Debug("RemoveQoS error during teardown",
			"error", err, "sw_if_index", swIfIndex)
	}

	if sg.ACLIngress != "" {
		if err := sb.RemoveIngressACL(swIfIndex); err != nil {
			log.Debug("RemoveIngressACL error during teardown",
				"error", err, "sw_if_index", swIfIndex)
		}
	}
	if sg.ACLEgress != "" {
		if err := sb.RemoveEgressACL(swIfIndex); err != nil {
			log.Debug("RemoveEgressACL error during teardown",
				"error", err, "sw_if_index", swIfIndex)
		}
	}

	if sg.URPF != "" && sg.URPF != "off" {
		if err := sb.DisableSourceVerify(swIfIndex); err != nil {
			log.Debug("DisableSourceVerify error during teardown",
				"error", err, "sw_if_index", swIfIndex)
		}
	}
}
