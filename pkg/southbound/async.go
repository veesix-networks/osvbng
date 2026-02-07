package southbound

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
)

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

	streamMu sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	logger *slog.Logger

	requestsSent   atomic.Uint64
	repliesRecv    atomic.Uint64
	errors         atomic.Uint64
	reconnects     atomic.Uint64
	queueHighWater atomic.Int64
}

type AsyncWorkerConfig struct {
	RequestQueueSize int
	RequestBufSize   int
	ReplyBufSize     int
}

func DefaultAsyncWorkerConfig() AsyncWorkerConfig {
	return AsyncWorkerConfig{
		RequestQueueSize: 10000,
		RequestBufSize:   1024,
		ReplyBufSize:     1024,
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
		conn:    conn,
		stream:  stream,
		cfg:     cfg,
		reqChan: make(chan *AsyncRequest, cfg.RequestQueueSize),
		pending: list.New(),
		ctx:     ctx,
		cancel:  cancel,
		logger:  logger.Get("vpp-async"),
	}

	return w, nil
}

func (w *AsyncWorker) Start() {
	w.wg.Add(2)
	go w.sendLoop()
	go w.recvLoop()
	w.logger.Info("VPP async worker started")
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
		"reconnects", w.reconnects.Load())
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
			req.Callback(nil, fmt.Errorf("connection reset"))
		}
	}
	w.pendingMu.Unlock()

	backoff := time.Duration(100) * time.Millisecond
	reconnectCount := w.reconnects.Load()
	if reconnectCount > 0 {
		backoff = time.Duration(min(reconnectCount*100, 2000)) * time.Millisecond
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
		return fmt.Errorf("create stream: %w", err)
	}

	w.stream = stream
	w.reconnects.Add(1)
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
			depth := len(w.reqChan)
			if int64(depth) > w.queueHighWater.Load() {
				w.queueHighWater.Store(int64(depth))
			}

			w.streamMu.Lock()
			err := w.stream.SendMsg(req.Message)
			w.streamMu.Unlock()

			if err != nil {
				w.errors.Add(1)
				w.logger.Error("Failed to send VPP message",
					"msg_type", req.Message.GetMessageName(),
					"error", err)

				if reconnErr := w.reconnect(); reconnErr != nil {
					w.logger.Error("Failed to reconnect VPP stream", "error", reconnErr)
				}

				if req.Callback != nil {
					req.Callback(nil, fmt.Errorf("send failed: %w", err))
				}
				continue
			}

			w.pendingMu.Lock()
			w.pending.PushBack(req)
			w.pendingMu.Unlock()
			w.requestsSent.Add(1)

			w.logger.Debug("Sent VPP request",
				"msg_type", req.Message.GetMessageName())
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

func (w *AsyncWorker) Metrics() map[string]uint64 {
	return map[string]uint64{
		"requests_sent":    w.requestsSent.Load(),
		"replies_received": w.repliesRecv.Load(),
		"errors":           w.errors.Load(),
		"queue_high_water": uint64(w.queueHighWater.Load()),
		"queue_current":    uint64(len(w.reqChan)),
	}
}
