package vpp

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"sync"

	"github.com/veesix-networks/osvbng/pkg/gomemif/memif"

	"github.com/veesix-networks/osvbng/pkg/dataplane"
	"github.com/veesix-networks/osvbng/pkg/logger"
)

type MemifEgress struct {
	socket    *memif.Socket
	iface     *memif.Interface
	txQueue   *memif.Queue
	logger    *slog.Logger
	mu        sync.Mutex
	connected bool
}

func NewMemifEgress() *MemifEgress {
	return &MemifEgress{
		logger: logger.Component(logger.ComponentEgress),
	}
}

func (m *MemifEgress) Init(socketPath string) error {
	if socketPath == "" {
		socketPath = "/run/vpp/memif.sock"
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
		// We need to re-evaluate and stress test the memif functionality for osvbng, it might not be the best option but it works pretty well for now...
		MemoryConfig: memif.MemoryConfig{
			NumQueuePairs: 1,
			Log2RingSize:  10,
		},
	}

	iface, err := socket.NewInterface(args)
	if err != nil {
		return fmt.Errorf("create memif interface: %w", err)
	}
	m.iface = iface

	go socket.StartPolling(nil)

	m.logger.Info("Connecting to VPP via memif", "socket", socketPath)
	if err := iface.RequestConnection(); err != nil {
		return fmt.Errorf("request connection: %w", err)
	}

	return nil
}

func (m *MemifEgress) onConnect(i *memif.Interface) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logger.Info("Memif connected to VPP")

	txq, err := i.GetTxQueue(0)
	if err != nil {
		return fmt.Errorf("get tx queue: %w", err)
	}
	m.txQueue = txq
	m.connected = true

	return nil
}

func (m *MemifEgress) onDisconnect(i *memif.Interface) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logger.Warn("Memif disconnected from VPP")
	m.connected = false
	m.txQueue = nil

	return nil
}

func (m *MemifEgress) SendPacket(pkt *dataplane.EgressPacket) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.connected || m.txQueue == nil {
		return fmt.Errorf("memif not connected")
	}

	frame := m.buildFrame(pkt)

	m.logger.Info("Sending frame via memif",
		"dst_mac", pkt.DstMAC,
		"src_mac", pkt.SrcMAC,
		"svlan", pkt.OuterVLAN,
		"cvlan", pkt.InnerVLAN,
		"frame_len", len(frame),
	)

	n := m.txQueue.WritePacket(frame)
	if n == 0 {
		return fmt.Errorf("failed to write packet to memif (queue full or error)")
	}
	if n < len(frame) {
		m.logger.Warn("Partial packet write", "wrote", n, "total", len(frame))
	}

	return nil
}

func (m *MemifEgress) buildFrame(pkt *dataplane.EgressPacket) []byte {
	frame := make([]byte, 0, 1500)

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
		etherType = 0x0800
	}
	frame = append(frame, byte(etherType>>8), byte(etherType))
	frame = append(frame, pkt.Payload...)

	return frame
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
