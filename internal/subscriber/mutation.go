// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package subscriber

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/veesix-networks/osvbng/pkg/aaa"
	"github.com/veesix-networks/osvbng/pkg/events"
)

const (
	defaultMutationTimeout   = 10 * time.Second
	defaultBurstThreshold    = 32
	defaultBucketSize        = 32
	defaultBucketInterval    = 10 * time.Millisecond
	defaultMaxTargetsPerCall = 10240

	ErrorCauseUnsupportedAttribute = 401
	ErrorCauseMissingAttribute     = 402
	ErrorCauseInvalidRequest       = 404
	ErrorCauseSessionNotFound      = 503
	ErrorCauseResourcesUnavailable = 506
)

type Target struct {
	SessionID     string `json:"session_id,omitempty"`
	AcctSessionID string `json:"acct_session_id,omitempty"`
	Username      string `json:"username,omitempty"`
	FramedIPv4    string `json:"framed_ipv4,omitempty"`
	FramedIPv6    string `json:"framed_ipv6,omitempty"`
}

type MutationResult struct {
	Mutated int            `json:"mutated"`
	Failed  int            `json:"failed"`
	Results []TargetResult `json:"results"`
}

type TargetResult struct {
	SessionID  string `json:"session_id"`
	Target     Target `json:"target"`
	Ok         bool   `json:"ok"`
	Error      string `json:"error,omitempty"`
	ErrorCause int    `json:"error_cause"`
}

type mutationWaiter struct {
	ch       chan TargetResult
	expected int
}

var allowedMutationAttrs = map[string]struct{}{
	aaa.AttrSessionTimeout:      {},
	aaa.AttrIdleTimeout:         {},
	aaa.AttrAcctInterimInterval: {},
	aaa.AttrACLIngress:          {},
	aaa.AttrACLEgress:           {},
	aaa.AttrQoSIngressPolicy:    {},
	aaa.AttrQoSEgressPolicy:     {},
	aaa.AttrQoSUploadRate:       {},
	aaa.AttrQoSDownloadRate:     {},
	aaa.AttrRateLimitUp:         {},
	aaa.AttrRateLimitDown:       {},
}

func validateAttributes(attrs map[string]string) (int, error) {
	if len(attrs) == 0 {
		return ErrorCauseMissingAttribute, fmt.Errorf("attribute delta is empty")
	}
	for k := range attrs {
		if _, ok := allowedMutationAttrs[k]; !ok {
			return ErrorCauseUnsupportedAttribute, fmt.Errorf("unsupported attribute: %s", k)
		}
	}
	return 0, nil
}

func validateTarget(t Target) (int, error) {
	count := 0
	if t.SessionID != "" {
		count++
	}
	if t.AcctSessionID != "" {
		count++
	}
	if t.Username != "" {
		count++
	}
	if t.FramedIPv4 != "" {
		count++
	}
	if t.FramedIPv6 != "" {
		count++
	}
	if count == 0 {
		return ErrorCauseMissingAttribute, fmt.Errorf("target has no identifier fields set")
	}
	if count > 1 {
		return ErrorCauseInvalidRequest, fmt.Errorf("target has multiple identifier fields set")
	}
	return 0, nil
}

func (c *Component) MutateSubscribers(ctx context.Context, targets []Target, attrs map[string]string) (*MutationResult, error) {
	if len(targets) > defaultMaxTargetsPerCall {
		return nil, fmt.Errorf("too many targets (%d > %d)", len(targets), defaultMaxTargetsPerCall)
	}

	if errCause, err := validateAttributes(attrs); err != nil {
		return &MutationResult{
			Failed:  len(targets),
			Results: makeFailedResults(targets, errCause, err.Error()),
		}, nil
	}

	var valid []Target
	result := &MutationResult{}

	for _, t := range targets {
		if errCause, err := validateTarget(t); err != nil {
			result.Results = append(result.Results, TargetResult{
				Target:     t,
				Ok:         false,
				Error:      err.Error(),
				ErrorCause: errCause,
			})
			result.Failed++
			continue
		}
		valid = append(valid, t)
	}

	if len(valid) == 0 {
		return result, nil
	}

	requestID := uuid.NewString()
	waiter := &mutationWaiter{
		ch:       make(chan TargetResult, len(valid)),
		expected: len(valid),
	}
	c.mutationWaiters.Store(requestID, waiter)
	defer c.mutationWaiters.Delete(requestID)

	c.publishMutationEvents(requestID, valid, attrs)

	timer := time.NewTimer(defaultMutationTimeout)
	defer timer.Stop()

	collected := 0
	for collected < len(valid) {
		select {
		case tr := <-waiter.ch:
			result.Results = append(result.Results, tr)
			if tr.Ok {
				result.Mutated++
			} else {
				result.Failed++
			}
			collected++
		case <-timer.C:
			for _, t := range valid {
				if !c.resultCollectedForTarget(result, t) {
					result.Results = append(result.Results, TargetResult{
						Target:     t,
						Ok:         false,
						Error:      "mutation timeout — applied state unknown",
						ErrorCause: ErrorCauseResourcesUnavailable,
					})
					result.Failed++
				}
			}
			return result, nil
		case <-ctx.Done():
			for _, t := range valid {
				if !c.resultCollectedForTarget(result, t) {
					result.Results = append(result.Results, TargetResult{
						Target:     t,
						Ok:         false,
						Error:      "context cancelled",
						ErrorCause: ErrorCauseResourcesUnavailable,
					})
					result.Failed++
				}
			}
			return result, nil
		}
	}

	return result, nil
}

func (c *Component) resultCollectedForTarget(result *MutationResult, t Target) bool {
	for _, r := range result.Results {
		if r.Target == t {
			return true
		}
	}
	return false
}

func targetToMutationEvent(requestID string, t Target, attrs map[string]string) *events.SubscriberMutationEvent {
	return &events.SubscriberMutationEvent{
		RequestID:      requestID,
		SessionID:      t.SessionID,
		AcctSessionID:  t.AcctSessionID,
		Username:       t.Username,
		FramedIPv4:     t.FramedIPv4,
		FramedIPv6:     t.FramedIPv6,
		AttributeDelta: attrs,
	}
}

func (c *Component) publishMutationEvents(requestID string, targets []Target, attrs map[string]string) {
	publish := func(t Target) {
		c.eventBus.Publish(events.TopicSubscriberMutation, events.Event{
			Source:    c.Name(),
			Timestamp: time.Now(),
			Data:      targetToMutationEvent(requestID, t, attrs),
		})
	}

	if len(targets) <= defaultBurstThreshold {
		for _, t := range targets {
			publish(t)
		}
		return
	}

	for i := 0; i < len(targets); i += defaultBucketSize {
		end := i + defaultBucketSize
		if end > len(targets) {
			end = len(targets)
		}
		bucket := targets[i:end]

		if i == 0 {
			for _, t := range bucket {
				publish(t)
			}
			continue
		}

		delay := time.Duration(i/defaultBucketSize) * defaultBucketInterval
		bucketCopy := make([]Target, len(bucket))
		copy(bucketCopy, bucket)
		time.AfterFunc(delay, func() {
			for _, t := range bucketCopy {
				publish(t)
			}
		})
	}
}

func (c *Component) MutateSession(ctx context.Context, sessionID string, attrs map[string]string) (*TargetResult, error) {
	result, err := c.MutateSubscribers(ctx, []Target{{SessionID: sessionID}}, attrs)
	if err != nil {
		return nil, err
	}
	if len(result.Results) == 0 {
		return nil, fmt.Errorf("no result returned")
	}
	return &result.Results[0], nil
}

func (c *Component) handleMutationResult(ev events.Event) {
	data, ok := ev.Data.(*events.SubscriberMutationResultEvent)
	if !ok {
		return
	}

	if data.Ok && data.Session != nil {
		if err := c.persistSession(data.Session); err != nil {
			c.logger.Warn("Failed to persist mutated session", "session_id", data.SessionID, "error", err)
		}
	}

	val, ok := c.mutationWaiters.Load(data.RequestID)
	if !ok {
		return
	}
	waiter := val.(*mutationWaiter)

	select {
	case waiter.ch <- TargetResult{
		SessionID:  data.SessionID,
		Ok:         data.Ok,
		Error:      data.Error,
		ErrorCause: data.ErrorCause,
	}:
	default:
	}
}

func makeFailedResults(targets []Target, errCause int, errMsg string) []TargetResult {
	results := make([]TargetResult, len(targets))
	for i, t := range targets {
		results[i] = TargetResult{
			Target:     t,
			Ok:         false,
			Error:      errMsg,
			ErrorCause: errCause,
		}
	}
	return results
}
