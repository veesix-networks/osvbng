// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"strconv"
	"sync/atomic"
	"time"
)

const (
	defaultSubscribeBufferSize = 256
	defaultTickInterval        = 1 * time.Second
)

// SubscribeOptions controls Subscribe behavior.
type SubscribeOptions struct {
	// PathGlob filters which metrics this subscriber receives updates for.
	// Empty or "*" matches all.
	PathGlob string

	// BufferSize is the per-subscriber update channel capacity. Default 256.
	// On overflow, updates are dropped and the subscription's drop counter
	// is incremented.
	BufferSize int

	// IncludeStreamingOnly controls whether streaming_only metrics are
	// published to this subscriber. Default false (Prometheus-safe).
	IncludeStreamingOnly bool
}

// Update is one tick's-worth of state for a single series. Subscribers
// receive Updates for every series of every metric that was marked dirty
// since the last tick.
type Update struct {
	Sample
	Timestamp time.Time
}

// Subscription is the handle returned by Subscribe. Updates are received
// from Updates(); call Unsubscribe to detach.
type Subscription struct {
	id             uint64
	registry       *Registry
	opts           SubscribeOptions
	updates        chan Update
	dropped        atomic.Uint64
	internalLabels []LabelPair

	unsubscribed atomic.Bool
}

// Updates returns the receive channel for telemetry updates.
func (s *Subscription) Updates() <-chan Update {
	return s.updates
}

// Dropped returns the cumulative count of updates dropped due to channel
// overflow on this subscription.
func (s *Subscription) Dropped() uint64 {
	return s.dropped.Load()
}

// Unsubscribe detaches the subscription from the registry. Safe to call
// multiple times. Buffered updates remain readable until drained.
func (s *Subscription) Unsubscribe() {
	if s.unsubscribed.Swap(true) {
		return
	}
	s.registry.subscribers.Delete(s.id)
	s.registry.subscriberCount.Add(-1)
	s.registry.maybeStopTick()
}

// Subscribe registers a subscriber and returns its Subscription. The first
// subscriber starts the registry's tick goroutine; the last to unsubscribe
// stops it.
func (r *Registry) Subscribe(opts SubscribeOptions) *Subscription {
	if opts.BufferSize <= 0 {
		opts.BufferSize = defaultSubscribeBufferSize
	}

	id := r.nextSubscriberID.Add(1)
	sub := &Subscription{
		id:             id,
		registry:       r,
		opts:           opts,
		updates:        make(chan Update, opts.BufferSize),
		internalLabels: []LabelPair{{Name: internalLabelSubscriber, Value: strconv.FormatUint(id, 10)}},
	}
	r.subscribers.Store(id, sub)
	r.subscriberCount.Add(1)
	r.maybeStartTick()
	return sub
}

// publish dispatches one update to one subscriber, applying that
// subscription's filters and drop-on-overflow policy.
func (s *Subscription) publish(sample Sample, ts time.Time) {
	if s.unsubscribed.Load() {
		return
	}
	if !s.opts.IncludeStreamingOnly && sample.StreamingOnly {
		return
	}
	if !MatchGlob(s.opts.PathGlob, sample.Name) {
		return
	}
	select {
	case s.updates <- Update{Sample: sample, Timestamp: ts}:
	default:
		s.dropped.Add(1)
	}
}
