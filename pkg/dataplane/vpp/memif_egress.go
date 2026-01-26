package vpp

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/veesix-networks/osvbng/pkg/dataplane"
	"github.com/veesix-networks/osvbng/pkg/ethernet"
	"github.com/veesix-networks/osvbng/pkg/gomemif/memif"
	"github.com/veesix-networks/osvbng/pkg/logger"
)

type MemifEgress struct {
	socket    *memif.Socket
	iface     *memif.Interface
	txQueues  []*memif.Queue
	queueMu   []sync.Mutex
	numQueues int
	nextQueue atomic.Uint32
	logger    *slog.Logger
	mu        sync.RWMutex
	connected bool
}

var bufferPool = sync.Pool{
	New: func() interface{} {
		return make([]memif.MemifPacketBuffer, 0, 256)
	},
}

var framePool = sync.Pool{
	New: func() interface{} {
		return make([]byte, 0, 1500)
	},
}

func NewMemifEgress() *MemifEgress {
	return &MemifEgress{
		logger: logger.Component(logger.ComponentEgress),
	}
}

func (m *MemifEgress) Init(socketPath string) error {
	if socketPath == "" {
		socketPath = "/run/osvbng/memif.sock"
	}

	socket, err := memif.NewSocket("osvbng", socketPath)
	if err != nil {
		return fmt.Errorf("create memif socket: %w", err)
	}
	m.socket = socket

	args := &memif.Arguments{
		IsMaster:         false,
		ConnectedFunc:    m.onConnect,
		DisconnectedFunc: m.onDisconnect,
		Name:             "osvbng-cp",
		MemoryConfig: memif.MemoryConfig{
			NumQueuePairs: 4,
			Log2RingSize:  12,
		},
	}

	iface, err := socket.NewInterface(args)
	if err != nil {
		return fmt.Errorf("create memif interface: %w", err)
	}
	m.iface = iface

	go socket.StartPolling(nil)

	m.logger.Info("Connecting to VPP via memif",
		"socket", socketPath,
		"queue_pairs", args.MemoryConfig.NumQueuePairs,
		"ring_size", 1<<args.MemoryConfig.Log2RingSize)
	if err := iface.RequestConnection(); err != nil {
		return fmt.Errorf("request connection: %w", err)
	}

	return nil
}

func (m *MemifEgress) onConnect(i *memif.Interface) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := i.GetMemoryConfig()
	m.numQueues = int(cfg.NumQueuePairs)
	m.txQueues = make([]*memif.Queue, m.numQueues)
	m.queueMu = make([]sync.Mutex, m.numQueues)

	for qid := 0; qid < m.numQueues; qid++ {
		txq, err := i.GetTxQueue(qid)
		if err != nil {
			return fmt.Errorf("get tx queue %d: %w", qid, err)
		}
		m.txQueues[qid] = txq
	}

	m.connected = true
	m.logger.Info("Memif connected to VPP", "queues", m.numQueues)

	return nil
}

func (m *MemifEgress) onDisconnect(i *memif.Interface) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logger.Warn("Memif disconnected from VPP")
	m.connected = false
	m.txQueues = nil
	m.numQueues = 0

	return nil
}

func (m *MemifEgress) SendPacket(pkt *dataplane.EgressPacket) error {
	m.mu.RLock()
	if !m.connected || m.numQueues == 0 {
		m.mu.RUnlock()
		return fmt.Errorf("memif not connected")
	}

	queueIdx := int(m.nextQueue.Add(1) % uint32(m.numQueues))
	txQueue := m.txQueues[queueIdx]
	m.mu.RUnlock()

	frame := m.buildFrame(pkt)

	m.queueMu[queueIdx].Lock()
	n := txQueue.WritePacket(frame)
	m.queueMu[queueIdx].Unlock()

	if n == 0 {
		return fmt.Errorf("failed to write packet to memif (queue full or error)")
	}
	if n < len(frame) {
		m.logger.Warn("Partial packet write", "wrote", n, "total", len(frame))
	}

	return nil
}

func (m *MemifEgress) SendPacketBatch(pkts []*dataplane.EgressPacket) (int, error) {
	if len(pkts) == 0 {
		return 0, nil
	}

	m.mu.RLock()
	if !m.connected || m.numQueues == 0 {
		m.mu.RUnlock()
		return 0, fmt.Errorf("memif not connected")
	}

	queueIdx := int(m.nextQueue.Add(1) % uint32(m.numQueues))
	txQueue := m.txQueues[queueIdx]
	m.mu.RUnlock()

	bufs := bufferPool.Get().([]memif.MemifPacketBuffer)
	bufs = bufs[:0]

	for _, pkt := range pkts {
		frame := m.buildFrame(pkt)
		bufs = append(bufs, memif.MemifPacketBuffer{
			Buf:    frame,
			Buflen: len(frame),
		})
	}

	m.queueMu[queueIdx].Lock()
	sent := txQueue.Tx_burst(bufs)
	m.queueMu[queueIdx].Unlock()

	bufferPool.Put(bufs)

	if sent == 0 && len(pkts) > 0 {
		return 0, fmt.Errorf("memif ring full")
	}

	return len(pkts), nil
}

func (m *MemifEgress) buildFrame(pkt *dataplane.EgressPacket) []byte {
	frame := framePool.Get().([]byte)
	frame = frame[:0]

	frame = append(frame, pkt.DstMAC...)
	frame = append(frame, pkt.SrcMAC...)

	if pkt.OuterVLAN != 0 {
		frame = append(frame, 0x81, 0x00)
		vlanTag := make([]byte, 2)
		binary.BigEndian.PutUint16(vlanTag, pkt.OuterVLAN)
		frame = append(frame, vlanTag...)

		if pkt.InnerVLAN != 0 {
			frame = append(frame, 0x81, 0x00)
			vlanTag := make([]byte, 2)
			binary.BigEndian.PutUint16(vlanTag, pkt.InnerVLAN)
			frame = append(frame, vlanTag...)
		}
	}

	etherType := pkt.EtherType
	if etherType == 0 {
		etherType = ethernet.EtherTypeIPv4
	}
	frame = append(frame, byte(etherType>>8), byte(etherType))
	frame = append(frame, pkt.Payload...)

	result := make([]byte, len(frame))
	copy(result, frame)

	framePool.Put(frame)

	return result
}

func (m *MemifEgress) Close() error {
	if m.iface != nil {
		if err := m.iface.Delete(); err != nil {
			return fmt.Errorf("delete interface: %w", err)
		}
	}
	if m.socket != nil {
		m.socket.Delete()
	}
	return nil
}
