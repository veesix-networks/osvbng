package local

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
)

type publishRequest struct {
	topic string
	event models.Event
}

type Bus struct {
	ctx       context.Context
	cancel    context.CancelFunc
	handlers  map[string][]events.EventHandler
	mu        sync.RWMutex
	publishCh chan publishRequest
	logger    *slog.Logger
}

func NewBus() events.Bus {
	ctx, cancel := context.WithCancel(context.Background())

	b := &Bus{
		ctx:       ctx,
		cancel:    cancel,
		handlers:  make(map[string][]events.EventHandler),
		publishCh: make(chan publishRequest, 10000),
		logger:    logger.Component(logger.ComponentEvents),
	}

	go b.publishLoop()

	return b
}

func (b *Bus) Publish(topic string, event models.Event) error {
	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	select {
	case b.publishCh <- publishRequest{topic: topic, event: event}:
		return nil
	default:
		return fmt.Errorf("publish channel full, dropping event")
	}
}

func (b *Bus) publishLoop() {
	for {
		select {
		case <-b.ctx.Done():
			return
		case req := <-b.publishCh:
			b.mu.RLock()
			handlers := b.handlers[req.topic]
			b.mu.RUnlock()

			for _, handler := range handlers {
				go func(h events.EventHandler, e models.Event) {
					if err := h(e); err != nil {
						b.logger.Error("Handler error", "topic", req.topic, "error", err)
					}
				}(handler, req.event)
			}
		}
	}
}

func (b *Bus) Subscribe(topic string, handler events.EventHandler) error {
	b.mu.Lock()
	b.handlers[topic] = append(b.handlers[topic], handler)
	handlerCount := len(b.handlers[topic])
	b.mu.Unlock()

	b.logger.Info("Subscribed to topic", "topic", topic, "handler_count", handlerCount)

	return nil
}

func (b *Bus) Unsubscribe(topic string, handler events.EventHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()

	handlers := b.handlers[topic]
	for i, h := range handlers {
		if &h == &handler {
			b.handlers[topic] = append(handlers[:i], handlers[i+1:]...)
			break
		}
	}
}

func (b *Bus) Close() error {
	b.cancel()
	return nil
}
