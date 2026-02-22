package local

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/logger"
)

type publishRequest struct {
	topic string
	event events.Event
}

type subscription struct {
	id      uint64
	handler events.Handler
}

type sub struct {
	bus   *Bus
	topic string
	id    uint64
}

func (s *sub) Unsubscribe() {
	s.bus.removeSub(s.topic, s.id)
}

type globalSub struct {
	bus *Bus
	id  uint64
}

func (s *globalSub) Unsubscribe() {
	s.bus.removeGlobalSub(s.id)
}

type Bus struct {
	ctx          context.Context
	cancel       context.CancelFunc
	subs         map[string]map[uint64]*subscription
	globalSubs   map[uint64]*subscription
	mu           sync.RWMutex
	nextID       atomic.Uint64
	publishCh    chan publishRequest
	logger       *slog.Logger
	published    atomic.Uint64
	dropped      atomic.Uint64
	debugTopics  map[string]bool
	debugSub     events.Subscription
}

func NewBus() events.Bus {
	ctx, cancel := context.WithCancel(context.Background())

	b := &Bus{
		ctx:        ctx,
		cancel:     cancel,
		subs:       make(map[string]map[uint64]*subscription),
		globalSubs: make(map[uint64]*subscription),
		publishCh:  make(chan publishRequest, 10000),
		logger:     logger.Get(logger.Events),
	}

	go b.publishLoop()

	return b
}

func (b *Bus) Publish(topic string, event events.Event) {
	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	if event.Type == "" {
		event.Type = topic
	}

	select {
	case b.publishCh <- publishRequest{topic: topic, event: event}:
		b.published.Add(1)
	default:
		b.dropped.Add(1)
		b.logger.Warn("Publish channel full, dropping event", "topic", topic)
	}
}

func (b *Bus) publishLoop() {
	for {
		select {
		case <-b.ctx.Done():
			return
		case req := <-b.publishCh:
			b.mu.RLock()
			topicSubs := b.subs[req.topic]
			handlers := make([]events.Handler, 0, len(topicSubs)+len(b.globalSubs))
			for _, s := range topicSubs {
				handlers = append(handlers, s.handler)
			}
			for _, s := range b.globalSubs {
				handlers = append(handlers, s.handler)
			}
			b.mu.RUnlock()

			for _, h := range handlers {
				go h(req.event)
			}
		}
	}
}

func (b *Bus) Subscribe(topic string, handler events.Handler) events.Subscription {
	id := b.nextID.Add(1)

	b.mu.Lock()
	if b.subs[topic] == nil {
		b.subs[topic] = make(map[uint64]*subscription)
	}
	b.subs[topic][id] = &subscription{id: id, handler: handler}
	handlerCount := len(b.subs[topic])
	b.mu.Unlock()

	b.logger.Info("Subscribed to topic", "topic", topic, "handler_count", handlerCount)

	return &sub{bus: b, topic: topic, id: id}
}

func (b *Bus) SubscribeAll(handler events.Handler) events.Subscription {
	id := b.nextID.Add(1)

	b.mu.Lock()
	b.globalSubs[id] = &subscription{id: id, handler: handler}
	count := len(b.globalSubs)
	b.mu.Unlock()

	b.logger.Info("Subscribed to all topics", "global_subscriber_count", count)

	return &globalSub{bus: b, id: id}
}

func (b *Bus) removeSub(topic string, id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if topicSubs, ok := b.subs[topic]; ok {
		delete(topicSubs, id)
		if len(topicSubs) == 0 {
			delete(b.subs, topic)
		}
	}
}

func (b *Bus) removeGlobalSub(id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.globalSubs, id)
}

func (b *Bus) Stats() events.Stats {
	b.mu.RLock()
	defer b.mu.RUnlock()

	topics := make([]events.TopicStats, 0, len(b.subs))
	for topic, subs := range b.subs {
		topics = append(topics, events.TopicStats{
			Topic:       topic,
			Subscribers: len(subs),
		})
	}

	var debugTopics []string
	for t := range b.debugTopics {
		debugTopics = append(debugTopics, t)
	}

	return events.Stats{
		Topics:       topics,
		PublishChLen: len(b.publishCh),
		PublishChCap: cap(b.publishCh),
		Published:    b.published.Load(),
		Dropped:      b.dropped.Load(),
		DebugTopics:  debugTopics,
	}
}

func (b *Bus) SetDebugTopics(topics []string) {
	b.mu.Lock()

	if len(topics) == 0 {
		b.debugTopics = nil
		oldSub := b.debugSub
		b.debugSub = nil
		b.mu.Unlock()
		if oldSub != nil {
			oldSub.Unsubscribe()
		}
		b.logger.Info("Event debug logging disabled")
		return
	}

	b.debugTopics = make(map[string]bool, len(topics))
	for _, t := range topics {
		b.debugTopics[t] = true
	}

	needSub := b.debugSub == nil
	b.mu.Unlock()

	if needSub {
		sub := b.SubscribeAll(func(e events.Event) {
			b.mu.RLock()
			match := b.debugTopics[e.Type]
			b.mu.RUnlock()
			if match {
				b.logger.Info("Event", "topic", e.Type, "source", e.Source, "data", e.Data)
			}
		})
		b.mu.Lock()
		b.debugSub = sub
		b.mu.Unlock()
	}

	b.logger.Info("Event debug logging enabled", "topics", topics)
}

func (b *Bus) DebugTopics() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if len(b.debugTopics) == 0 {
		return nil
	}

	topics := make([]string, 0, len(b.debugTopics))
	for t := range b.debugTopics {
		topics = append(topics, t)
	}
	return topics
}

func (b *Bus) Close() error {
	b.cancel()
	return nil
}
