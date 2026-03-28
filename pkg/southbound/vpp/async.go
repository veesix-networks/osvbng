package vpp

import (
	"context"
	"errors"
	"fmt"
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
	reqChan chan *AsyncRequest
	cfg     AsyncWorkerConfig

	circuitOpen     atomic.Bool
	consecutiveFail atomic.Int32

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	logger *logger.Logger

	requestsSent      atomic.Uint64
	repliesRecv       atomic.Uint64
	errors            atomic.Uint64
	rejected          atomic.Uint64
	queueHighWater    atomic.Int64
	inflight          atomic.Int64
	replyTimeouts     atomic.Uint64
	streamRecreations atomic.Uint64
}

type AsyncWorkerConfig struct {
	PoolSize         int
	RequestQueueSize int
	ReplyTimeout     time.Duration
	RequestBufSize   int
	ReplyBufSize     int
}

func DefaultAsyncWorkerConfig() AsyncWorkerConfig {
	return AsyncWorkerConfig{
		PoolSize:         64,
		RequestQueueSize: 10000,
		ReplyTimeout:     5 * time.Second,
		RequestBufSize:   8,
		ReplyBufSize:     8,
	}
}

func NewAsyncWorker(conn *core.Connection, cfg AsyncWorkerConfig) (*AsyncWorker, error) {
	ctx, cancel := context.WithCancel(context.Background())

	w := &AsyncWorker{
		conn:    conn,
		cfg:     cfg,
		reqChan: make(chan *AsyncRequest, cfg.RequestQueueSize),
		ctx:     ctx,
		cancel:  cancel,
		logger:  logger.Get("vpp-async"),
	}

	return w, nil
}

func (w *AsyncWorker) Start() {
	w.wg.Add(w.cfg.PoolSize)
	for i := 0; i < w.cfg.PoolSize; i++ {
		go w.worker(i)
	}
	w.logger.Info("VPP async worker started", "pool_size", w.cfg.PoolSize, "reply_timeout", w.cfg.ReplyTimeout)
}

func (w *AsyncWorker) Stop() {
	w.circuitOpen.Store(true)
	w.cancel()
	w.wg.Wait()

	// Drain remaining requests
	for {
		select {
		case req := <-w.reqChan:
			if req.Callback != nil {
				go req.Callback(nil, ErrVPPUnavailable)
			}
		default:
			w.logger.Info("VPP async worker stopped",
				"requests_sent", w.requestsSent.Load(),
				"replies_recv", w.repliesRecv.Load(),
				"errors", w.errors.Load(),
				"rejected", w.rejected.Load(),
				"reply_timeouts", w.replyTimeouts.Load(),
				"stream_recreations", w.streamRecreations.Load())
			return
		}
	}
}

func (w *AsyncWorker) createStream(replyBufSize int) (api.Stream, error) {
	return w.conn.NewStream(w.ctx,
		core.WithRequestSize(w.cfg.RequestBufSize),
		core.WithReplySize(replyBufSize),
		core.WithReplyTimeout(w.cfg.ReplyTimeout),
	)
}

func (w *AsyncWorker) worker(id int) {
	defer w.wg.Done()

	stream, err := w.createStream(w.cfg.ReplyBufSize)
	if err != nil {
		w.logger.Error("Failed to create initial stream", "worker", id, "error", err)
		fails := w.consecutiveFail.Add(1)
		if fails >= 3 {
			w.circuitOpen.Store(true)
			w.logger.Error("VPP connection lost - circuit breaker OPEN", "consecutive_failures", fails)
		}
		return
	}
	defer stream.Close() //nolint:errcheck

	recycleCount := 0

	for {
		select {
		case <-w.ctx.Done():
			return
		case req, ok := <-w.reqChan:
			if !ok {
				return
			}

			if w.circuitOpen.Load() {
				w.rejected.Add(1)
				if req.Callback != nil {
					go req.Callback(nil, ErrVPPUnavailable)
				}
				continue
			}

			depth := len(w.reqChan)
			if int64(depth) > w.queueHighWater.Load() {
				w.queueHighWater.Store(int64(depth))
			}

			w.inflight.Add(1)

			err := stream.SendMsg(req.Message)
			if err != nil {
				w.inflight.Add(-1)
				w.errors.Add(1)
				w.logger.Error("Failed to send VPP message",
					"worker", id,
					"msg_type", req.Message.GetMessageName(),
					"error", err)
				if req.Callback != nil {
					go req.Callback(nil, fmt.Errorf("send %s: %w", req.Message.GetMessageName(), err))
				}
				stream.Close() //nolint:errcheck
				stream, recycleCount = w.recycleStream(id, recycleCount)
				if stream == nil {
					return
				}
				continue
			}

			w.requestsSent.Add(1)
			w.consecutiveFail.Store(0)

			reply, err := stream.RecvMsg()

			w.inflight.Add(-1)

			if err != nil {
				w.errors.Add(1)
				if w.ctx.Err() != nil {
					if req.Callback != nil {
						go req.Callback(nil, ErrVPPUnavailable)
					}
					return
				}
				isTimeout := isReplyTimeout(err)
				if isTimeout {
					w.replyTimeouts.Add(1)
					w.logger.Warn("VPP reply timeout",
						"worker", id,
						"msg_type", req.Message.GetMessageName(),
						"timeout", w.cfg.ReplyTimeout)
				} else {
					w.logger.Error("Failed to receive VPP reply",
						"worker", id,
						"msg_type", req.Message.GetMessageName(),
						"error", err)
				}
				if req.Callback != nil {
					go req.Callback(nil, fmt.Errorf("recv %s: %w", req.Message.GetMessageName(), err))
				}
				stream.Close() //nolint:errcheck
				stream, recycleCount = w.recycleStream(id, recycleCount)
				if stream == nil {
					return
				}
				continue
			}

			w.repliesRecv.Add(1)
			if req.Callback != nil {
				go req.Callback(reply, nil)
			}
		}
	}
}

func (w *AsyncWorker) recycleStream(id int, recycleCount int) (api.Stream, int) {
	recycleCount++
	w.streamRecreations.Add(1)

	// Alternate reply buffer size to force govpp to allocate a fresh replyChan,
	// preventing stale replies from a pooled channel from contaminating the next request.
	replyBuf := w.cfg.ReplyBufSize + (recycleCount % 2)

	stream, err := w.createStream(replyBuf)
	if err != nil {
		fails := w.consecutiveFail.Add(1)
		if fails >= 3 && !w.circuitOpen.Load() {
			w.circuitOpen.Store(true)
			w.logger.Error("VPP connection lost - circuit breaker OPEN",
				"worker", id, "consecutive_failures", fails)
		}
		w.logger.Error("Failed to recreate stream",
			"worker", id, "error", err)
		return nil, recycleCount
	}

	w.consecutiveFail.Store(0)
	if w.circuitOpen.Swap(false) {
		w.logger.Info("VPP connection restored - circuit breaker CLOSED", "worker", id)
	}
	w.logger.Warn("Stream recycled", "worker", id, "recycle_count", recycleCount)
	return stream, recycleCount
}

func (w *AsyncWorker) SendAsync(msg api.Message, callback func(api.Message, error)) error {
	if w.circuitOpen.Load() {
		w.rejected.Add(1)
		if callback != nil {
			go callback(nil, ErrVPPUnavailable)
		}
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
		err := fmt.Errorf("VPP request queue full")
		if callback != nil {
			go callback(nil, err)
		}
		return err
	}
}

func (w *AsyncWorker) IsAvailable() bool {
	return !w.circuitOpen.Load()
}

func (w *AsyncWorker) Metrics() map[string]uint64 {
	return map[string]uint64{
		"requests_sent":      w.requestsSent.Load(),
		"replies_received":   w.repliesRecv.Load(),
		"errors":             w.errors.Load(),
		"queue_high_water":   uint64(w.queueHighWater.Load()),
		"queue_current":      uint64(len(w.reqChan)),
		"inflight_current":   uint64(w.inflight.Load()),
		"rejected":           w.rejected.Load(),
		"reply_timeouts":     w.replyTimeouts.Load(),
		"stream_recreations": w.streamRecreations.Load(),
	}
}

func isReplyTimeout(err error) bool {
	return errors.Is(err, core.ErrReplyTimeout)
}
