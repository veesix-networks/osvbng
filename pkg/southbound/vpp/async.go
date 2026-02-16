package vpp

import (
	"container/list"
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"go.fd.io/govpp/api"
	"go.fd.io/govpp/core"

	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

var ErrVPPUnavailable = southbound.ErrUnavailable

type AsyncRequest struct {
	Message  api.Message
	Callback func(api.Message, error)
}

type AsyncWorker struct {
	conn    *core.Connection
	stream  api.Stream
	reqChan chan *AsyncRequest
	cfg     AsyncWorkerConfig

	pendingMu sync.Mutex
	pending   *list.List

	inflight chan struct{}

	streamMu sync.Mutex

	circuitOpen     atomic.Bool
	consecutiveFail atomic.Int32

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	logger *slog.Logger

	requestsSent   atomic.Uint64
	repliesRecv    atomic.Uint64
	errors         atomic.Uint64
	reconnects     atomic.Uint64
	rejected       atomic.Uint64
	queueHighWater atomic.Int64
}

type AsyncWorkerConfig struct {
	RequestQueueSize int
	RequestBufSize   int
	ReplyBufSize     int
	MaxInflight      int
}

func DefaultAsyncWorkerConfig() AsyncWorkerConfig {
	return AsyncWorkerConfig{
		RequestQueueSize: 10000,
		RequestBufSize:   1024,
		ReplyBufSize:     1024,
		MaxInflight:      256,
	}
}

func NewAsyncWorker(conn *core.Connection, cfg AsyncWorkerConfig) (*AsyncWorker, error) {
	ctx, cancel := context.WithCancel(context.Background())

	stream, err := conn.NewStream(ctx,
		core.WithRequestSize(cfg.RequestBufSize),
		core.WithReplySize(cfg.ReplyBufSize),
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create stream: %w", err)
	}

	w := &AsyncWorker{
		conn:     conn,
		stream:   stream,
		cfg:      cfg,
		reqChan:  make(chan *AsyncRequest, cfg.RequestQueueSize),
		pending:  list.New(),
		inflight: make(chan struct{}, cfg.MaxInflight),
		ctx:      ctx,
		cancel:   cancel,
		logger:   logger.Get("vpp-async"),
	}

	return w, nil
}

func (w *AsyncWorker) Start() {
	w.wg.Add(2)
	go w.sendLoop()
	go w.recvLoop()
	w.logger.Info("VPP async worker started", "max_inflight", w.cfg.MaxInflight)
}

func (w *AsyncWorker) Stop() {
	w.cancel()
	w.wg.Wait()
	w.streamMu.Lock()
	w.stream.Close()
	w.streamMu.Unlock()
	w.logger.Info("VPP async worker stopped",
		"requests_sent", w.requestsSent.Load(),
		"replies_recv", w.repliesRecv.Load(),
		"errors", w.errors.Load(),
		"reconnects", w.reconnects.Load(),
		"rejected", w.rejected.Load())
}

func (w *AsyncWorker) reconnect() error {
	w.streamMu.Lock()
	defer w.streamMu.Unlock()

	w.stream.Close()

	w.pendingMu.Lock()
	dropped := w.pending.Len()
	for elem := w.pending.Front(); elem != nil; elem = w.pending.Front() {
		w.pending.Remove(elem)
		req := elem.Value.(*AsyncRequest)
		if req.Callback != nil {
			req.Callback(nil, ErrVPPUnavailable)
		}
		<-w.inflight
	}
	w.pendingMu.Unlock()

	backoff := time.Duration(100) * time.Millisecond
	failCount := w.consecutiveFail.Load()
	if failCount > 0 {
		backoff = time.Duration(min(int64(failCount)*500, 5000)) * time.Millisecond
	}

	select {
	case <-w.ctx.Done():
		return w.ctx.Err()
	case <-time.After(backoff):
	}

	stream, err := w.conn.NewStream(w.ctx,
		core.WithRequestSize(w.cfg.RequestBufSize),
		core.WithReplySize(w.cfg.ReplyBufSize),
	)
	if err != nil {
		fails := w.consecutiveFail.Add(1)
		if fails >= 3 && !w.circuitOpen.Load() {
			w.circuitOpen.Store(true)
			w.logger.Error("VPP connection lost - circuit breaker OPEN, rejecting new requests",
				"consecutive_failures", fails, "dropped_pending", dropped)
		}
		return fmt.Errorf("create stream: %w", err)
	}

	w.stream = stream
	w.reconnects.Add(1)

	wasOpen := w.circuitOpen.Swap(false)
	w.consecutiveFail.Store(0)
	if wasOpen {
		w.logger.Info("VPP connection restored - circuit breaker CLOSED")
	}
	w.logger.Info("VPP async worker reconnected", "dropped_pending", dropped, "backoff_ms", backoff.Milliseconds())
	return nil
}

func (w *AsyncWorker) sendLoop() {
	defer w.wg.Done()

	for {
		select {
		case <-w.ctx.Done():
			return
		case req := <-w.reqChan:
			if w.circuitOpen.Load() {
				w.rejected.Add(1)
				if req.Callback != nil {
					req.Callback(nil, ErrVPPUnavailable)
				}
				continue
			}

			select {
			case <-w.ctx.Done():
				return
			case w.inflight <- struct{}{}:
			}

			depth := len(w.reqChan)
			if int64(depth) > w.queueHighWater.Load() {
				w.queueHighWater.Store(int64(depth))
			}

			w.streamMu.Lock()
			err := w.stream.SendMsg(req.Message)
			w.streamMu.Unlock()

			if err != nil {
				<-w.inflight

				w.errors.Add(1)
				w.logger.Error("Failed to send VPP message",
					"msg_type", req.Message.GetMessageName(),
					"error", err)

				if reconnErr := w.reconnect(); reconnErr != nil {
					w.logger.Debug("Reconnect failed", "error", reconnErr)
				}

				if req.Callback != nil {
					req.Callback(nil, fmt.Errorf("send failed: %w", err))
				}
				continue
			}

			w.consecutiveFail.Store(0)

			w.pendingMu.Lock()
			w.pending.PushBack(req)
			w.pendingMu.Unlock()
			w.requestsSent.Add(1)

			w.logger.Debug("Sent VPP request",
				"msg_type", req.Message.GetMessageName(),
				"inflight", len(w.inflight))
		}
	}
}

func (w *AsyncWorker) recvLoop() {
	defer w.wg.Done()

	for {
		select {
		case <-w.ctx.Done():
			return
		default:
			w.streamMu.Lock()
			stream := w.stream
			w.streamMu.Unlock()

			reply, err := stream.RecvMsg()
			if err != nil {
				if w.ctx.Err() != nil {
					return
				}
				w.errors.Add(1)
				w.logger.Debug("Receive error (stream may have reconnected)", "error", err)
				continue
			}

			w.repliesRecv.Add(1)

			w.logger.Debug("Received VPP reply",
				"msg_type", reply.GetMessageName())

			w.pendingMu.Lock()
			elem := w.pending.Front()
			if elem != nil {
				w.pending.Remove(elem)
				w.pendingMu.Unlock()

				<-w.inflight

				req := elem.Value.(*AsyncRequest)
				if req.Callback != nil {
					req.Callback(reply, nil)
				}
			} else {
				w.pendingMu.Unlock()
				w.logger.Warn("Received reply with no pending request",
					"msg_type", reply.GetMessageName())
			}
		}
	}
}

func (w *AsyncWorker) SendAsync(msg api.Message, callback func(api.Message, error)) error {
	if w.circuitOpen.Load() {
		w.rejected.Add(1)
		return ErrVPPUnavailable
	}

	req := &AsyncRequest{
		Message:  msg,
		Callback: callback,
	}

	select {
	case w.reqChan <- req:
		return nil
	default:
		w.errors.Add(1)
		return fmt.Errorf("VPP request queue full")
	}
}

func (w *AsyncWorker) IsAvailable() bool {
	return !w.circuitOpen.Load()
}

func (w *AsyncWorker) Metrics() map[string]uint64 {
	return map[string]uint64{
		"requests_sent":    w.requestsSent.Load(),
		"replies_received": w.repliesRecv.Load(),
		"errors":           w.errors.Load(),
		"queue_high_water": uint64(w.queueHighWater.Load()),
		"queue_current":    uint64(len(w.reqChan)),
		"inflight_current": uint64(len(w.inflight)),
		"rejected":         w.rejected.Load(),
	}
}
