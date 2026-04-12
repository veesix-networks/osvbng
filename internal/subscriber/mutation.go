// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package subscriber

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/veesix-networks/osvbng/pkg/aaa"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/models"
)

const (
	defaultMutationTimeout  = 10 * time.Second
	defaultBurstThreshold   = 32
	defaultBucketSize       = 32
	defaultBucketInterval   = 10 * time.Millisecond
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

type resolvedTarget struct {
	sessionID  string
	accessType models.AccessType
	target     Target
}

func (c *Component) resolveTarget(ctx context.Context, t Target) (*resolvedTarget, int, error) {
	var sessionID string
	var err error

	switch {
	case t.SessionID != "":
		sessionID = t.SessionID
	case t.AcctSessionID != "":
		sessionID, err = c.lookupByKey(ctx, fmt.Sprintf("osvbng:lookup:acct-session-id:%s", t.AcctSessionID))
	case t.Username != "":
		sessionID, err = c.lookupByKey(ctx, fmt.Sprintf("osvbng:lookup:username:%s", t.Username))
	case t.FramedIPv4 != "":
		sessionID, err = c.lookupByKey(ctx, fmt.Sprintf("osvbng:lookup:framed-ipv4:%s", t.FramedIPv4))
	case t.FramedIPv6 != "":
		sessionID, err = c.lookupByKey(ctx, fmt.Sprintf("osvbng:lookup:framed-ipv6:%s", t.FramedIPv6))
	}

	if err != nil {
		return nil, ErrorCauseSessionNotFound, fmt.Errorf("session not found")
	}

	if sessionID == "" {
		return nil, ErrorCauseSessionNotFound, fmt.Errorf("session not found")
	}

	sess, accessType, err := c.GetSessionGeneric(ctx, sessionID)
	if err != nil {
		return nil, ErrorCauseSessionNotFound, fmt.Errorf("session not found: %w", err)
	}

	if t.Username != "" {
		if err := c.checkUsernameAmbiguity(ctx, t.Username, sessionID); err != nil {
			return nil, ErrorCauseSessionNotFound, err
		}
	}

	if t.AcctSessionID != "" && sess.GetAAASessionID() != t.AcctSessionID {
		return nil, ErrorCauseSessionNotFound, fmt.Errorf("stale lookup key for acct-session-id")
	}
	if t.FramedIPv4 != "" && (sess.GetIPv4Address() == nil || sess.GetIPv4Address().String() != t.FramedIPv4) {
		return nil, ErrorCauseSessionNotFound, fmt.Errorf("stale lookup key for framed-ipv4")
	}

	return &resolvedTarget{
		sessionID:  sessionID,
		accessType: accessType,
		target:     t,
	}, 0, nil
}

func (c *Component) lookupByKey(ctx context.Context, key string) (string, error) {
	data, err := c.cache.Get(ctx, key)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", fmt.Errorf("empty lookup")
	}
	return string(data), nil
}

func (c *Component) checkUsernameAmbiguity(ctx context.Context, username, expectedSessionID string) error {
	pattern := "osvbng:sessions:*"
	var cursor uint64
	matchCount := 0

	for {
		keys, nextCursor, err := c.cache.Scan(ctx, cursor, pattern, 100)
		if err != nil {
			break
		}

		for _, key := range keys {
			data, err := c.cache.Get(ctx, key)
			if err != nil {
				continue
			}
			var meta struct {
				Username string `json:"Username"`
				State    string `json:"State"`
			}
			if err := json.Unmarshal(data, &meta); err != nil {
				continue
			}
			if meta.Username == username && meta.State == string(models.SessionStateActive) {
				matchCount++
				if matchCount > 1 {
					return fmt.Errorf("ambiguous username — use acct_session_id or session_id")
				}
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return nil
}

func (c *Component) GetSessionGeneric(ctx context.Context, sessionID string) (models.SubscriberSession, models.AccessType, error) {
	key := fmt.Sprintf("osvbng:sessions:%s", sessionID)
	data, err := c.cache.Get(ctx, key)
	if err != nil {
		return nil, "", fmt.Errorf("session not found: %s", sessionID)
	}
	if len(data) == 0 {
		return nil, "", fmt.Errorf("empty session data: %s", sessionID)
	}

	var meta struct {
		AccessType string `json:"AccessType"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, "", fmt.Errorf("unmarshal metadata: %w", err)
	}

	switch models.AccessType(meta.AccessType) {
	case models.AccessTypePPPoE:
		var sess models.PPPSession
		if err := json.Unmarshal(data, &sess); err != nil {
			return nil, "", fmt.Errorf("unmarshal ppp session: %w", err)
		}
		return &sess, models.AccessTypePPPoE, nil
	default:
		var sess models.IPoESession
		if err := json.Unmarshal(data, &sess); err != nil {
			return nil, "", fmt.Errorf("unmarshal ipoe session: %w", err)
		}
		return &sess, models.AccessTypeIPoE, nil
	}
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

	var resolved []*resolvedTarget
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

		rt, errCause, err := c.resolveTarget(ctx, t)
		if err != nil {
			result.Results = append(result.Results, TargetResult{
				Target:     t,
				Ok:         false,
				Error:      err.Error(),
				ErrorCause: errCause,
			})
			result.Failed++
			continue
		}

		resolved = append(resolved, rt)
	}

	if len(resolved) == 0 {
		return result, nil
	}

	requestID := uuid.NewString()
	waiter := &mutationWaiter{
		ch:       make(chan TargetResult, len(resolved)),
		expected: len(resolved),
	}
	c.mutationWaiters.Store(requestID, waiter)
	defer c.mutationWaiters.Delete(requestID)

	c.publishMutationEvents(requestID, resolved, attrs)

	timer := time.NewTimer(defaultMutationTimeout)
	defer timer.Stop()

	collected := 0
	for collected < len(resolved) {
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
			for _, rt := range resolved {
				if !c.resultCollected(result, rt.sessionID) {
					result.Results = append(result.Results, TargetResult{
						SessionID:  rt.sessionID,
						Target:     rt.target,
						Ok:         false,
						Error:      "mutation timeout — applied state unknown",
						ErrorCause: ErrorCauseResourcesUnavailable,
					})
					result.Failed++
				}
			}
			return result, nil
		case <-ctx.Done():
			for _, rt := range resolved {
				if !c.resultCollected(result, rt.sessionID) {
					result.Results = append(result.Results, TargetResult{
						SessionID:  rt.sessionID,
						Target:     rt.target,
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

func (c *Component) resultCollected(result *MutationResult, sessionID string) bool {
	for _, r := range result.Results {
		if r.SessionID == sessionID {
			return true
		}
	}
	return false
}

func (c *Component) publishMutationEvents(requestID string, resolved []*resolvedTarget, attrs map[string]string) {
	if len(resolved) <= defaultBurstThreshold {
		for _, rt := range resolved {
			c.eventBus.Publish(events.TopicSubscriberMutation, events.Event{
				Source:    c.Name(),
				Timestamp: time.Now(),
				Data: &events.SubscriberMutationEvent{
					RequestID:      requestID,
					SessionID:      rt.sessionID,
					AccessType:     rt.accessType,
					AttributeDelta: attrs,
				},
			})
		}
		return
	}

	for i := 0; i < len(resolved); i += defaultBucketSize {
		end := i + defaultBucketSize
		if end > len(resolved) {
			end = len(resolved)
		}
		bucket := resolved[i:end]

		if i == 0 {
			for _, rt := range bucket {
				c.eventBus.Publish(events.TopicSubscriberMutation, events.Event{
					Source:    c.Name(),
					Timestamp: time.Now(),
					Data: &events.SubscriberMutationEvent{
						RequestID:      requestID,
						SessionID:      rt.sessionID,
						AccessType:     rt.accessType,
						AttributeDelta: attrs,
					},
				})
			}
			continue
		}

		delay := time.Duration(i/defaultBucketSize) * defaultBucketInterval
		bucketCopy := make([]*resolvedTarget, len(bucket))
		copy(bucketCopy, bucket)
		time.AfterFunc(delay, func() {
			for _, rt := range bucketCopy {
				c.eventBus.Publish(events.TopicSubscriberMutation, events.Event{
					Source:    c.Name(),
					Timestamp: time.Now(),
					Data: &events.SubscriberMutationEvent{
						RequestID:      requestID,
						SessionID:      rt.sessionID,
						AccessType:     rt.accessType,
						AttributeDelta: attrs,
					},
				})
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
