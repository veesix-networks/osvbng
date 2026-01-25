package vpp

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/veesix-networks/osvbng/pkg/dataplane"
	"github.com/veesix-networks/osvbng/pkg/ethernet"
	"github.com/veesix-networks/osvbng/pkg/gomemif/memif"
	"github.com/veesix-networks/osvbng/pkg/logger"
)

type ARPPacket struct {
	SrcMAC    net.HardwareAddr
	DstMAC    net.HardwareAddr
	OuterVLAN uint16
	InnerVLAN uint16
	Operation uint16
	SenderMAC net.HardwareAddr
	SenderIP  net.IP
	TargetMAC net.HardwareAddr
	TargetIP  net.IP
}

type ARPHandler func(*ARPPacket)

type MemifHandler struct {
	socket     *memif.Socket
	iface      *memif.Interface
	rxQueue    *memif.Queue
	txQueue    *memif.Queue
	gatewayMAC net.HardwareAddr
	arpHandler ARPHandler
	logger     *slog.Logger
	mu         sync.Mutex
	connected  bool
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

func NewMemifHandler(virtualMAC string, arpHandler ARPHandler) (*MemifHandler, error) {
	var gwMAC net.HardwareAddr
	var err error

	if virtualMAC != "" {
		gwMAC, err = net.ParseMAC(virtualMAC)
		if err != nil {
			return nil, fmt.Errorf("parse virtual MAC: %w", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &MemifHandler{
		gatewayMAC: gwMAC,
		arpHandler: arpHandler,
		logger:     logger.Component("memif"),
		ctx:        ctx,
		cancel:     cancel,
	}, nil
}

func (m *MemifHandler) Init(socketPath string) error {
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

func (m *MemifHandler) onConnect(i *memif.Interface) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logger.Info("Memif connected to VPP")

	rxq, err := i.GetRxQueue(0)
	if err != nil {
		return fmt.Errorf("get rx queue: %w", err)
	}
	m.rxQueue = rxq

	txq, err := i.GetTxQueue(0)
	if err != nil {
		return fmt.Errorf("get tx queue: %w", err)
	}
	m.txQueue = txq

	i.Pkt = make([]memif.MemifPacketBuffer, 64)
	for idx := range i.Pkt {
		i.Pkt[idx].Buf = make([]byte, 2048)
		i.Pkt[idx].Buflen = 2048
	}

	rxq.Refill(0)

	m.connected = true

	m.wg.Add(1)
	go m.rxLoop()

	return nil
}

func (m *MemifHandler) onDisconnect(i *memif.Interface) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logger.Warn("Memif disconnected from VPP")
	m.connected = false
	m.rxQueue = nil
	m.txQueue = nil

	return nil
}

func (m *MemifHandler) rxLoop() {
	defer m.wg.Done()

	m.logger.Info("Started memif RX loop")

	for {
		select {
		case <-m.ctx.Done():
			return
		default:
			m.mu.Lock()
			if !m.connected || m.rxQueue == nil {
				m.mu.Unlock()
				return
			}

			pkt := m.iface.Pkt
			nPackets, err := m.rxQueue.Rx_burst(pkt)
			m.mu.Unlock()

			if err != nil {
				m.logger.Error("RX error", "error", err)
				continue
			}

			if nPackets == 0 {
				continue
			}

			for i := 0; i < int(nPackets); i++ {
				m.handleRxPacket(pkt[i].Buf[:pkt[i].Buflen])
			}

			m.mu.Lock()
			if m.rxQueue != nil {
				m.rxQueue.Refill(int(nPackets))
			}
			m.mu.Unlock()
		}
	}
}

func (m *MemifHandler) handleRxPacket(frame []byte) {
	if len(frame) < 14 {
		return
	}

	dstMAC := net.HardwareAddr(frame[0:6])
	srcMAC := net.HardwareAddr(frame[6:12])
	etherType := binary.BigEndian.Uint16(frame[12:14])

	offset := 14
	var outerVLAN, innerVLAN uint16

	if etherType == 0x8100 {
		if len(frame) < offset+4 {
			return
		}
		outerVLAN = binary.BigEndian.Uint16(frame[offset:offset+2]) & 0x0FFF
		etherType = binary.BigEndian.Uint16(frame[offset+2 : offset+4])
		offset += 4

		if etherType == 0x8100 {
			if len(frame) < offset+4 {
				return
			}
			innerVLAN = binary.BigEndian.Uint16(frame[offset:offset+2]) & 0x0FFF
			etherType = binary.BigEndian.Uint16(frame[offset+2 : offset+4])
			offset += 4
		}
	}

	if etherType != ethernet.EtherTypeARP {
		return
	}

	arp := m.parseARP(frame[offset:], srcMAC, dstMAC, outerVLAN, innerVLAN)
	if arp == nil {
		return
	}

	m.logger.Info("Received ARP via memif",
		"operation", arp.Operation,
		"sender_ip", arp.SenderIP,
		"target_ip", arp.TargetIP,
		"svlan", arp.OuterVLAN,
		"cvlan", arp.InnerVLAN,
	)

	if m.arpHandler != nil {
		m.arpHandler(arp)
	}
}

func (m *MemifHandler) parseARP(data []byte, srcMAC, dstMAC net.HardwareAddr, outerVLAN, innerVLAN uint16) *ARPPacket {
	if len(data) < 28 {
		return nil
	}

	hwType := binary.BigEndian.Uint16(data[0:2])
	protoType := binary.BigEndian.Uint16(data[2:4])
	hwLen := data[4]
	protoLen := data[5]
	operation := binary.BigEndian.Uint16(data[6:8])

	if hwType != 1 || protoType != ethernet.EtherTypeIPv4 || hwLen != 6 || protoLen != 4 {
		return nil
	}

	return &ARPPacket{
		SrcMAC:    srcMAC,
		DstMAC:    dstMAC,
		OuterVLAN: outerVLAN,
		InnerVLAN: innerVLAN,
		Operation: operation,
		SenderMAC: net.HardwareAddr(data[8:14]),
		SenderIP:  net.IP(data[14:18]),
		TargetMAC: net.HardwareAddr(data[18:24]),
		TargetIP:  net.IP(data[24:28]),
	}
}

func (m *MemifHandler) SendARPReply(arp *ARPPacket) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.connected || m.txQueue == nil {
		return fmt.Errorf("memif not connected")
	}

	frame := m.buildARPReply(arp)

	n := m.txQueue.WritePacket(frame)
	if n == 0 {
		return fmt.Errorf("failed to write ARP reply to memif")
	}

	m.logger.Info("Sent ARP reply via memif",
		"target_ip", arp.SenderIP,
		"target_mac", arp.SenderMAC,
		"svlan", arp.OuterVLAN,
		"cvlan", arp.InnerVLAN,
	)

	return nil
}

func (m *MemifHandler) buildARPReply(req *ARPPacket) []byte {
	frame := make([]byte, 0, 256)

	frame = append(frame, req.SenderMAC...)
	frame = append(frame, m.gatewayMAC...)

	if req.OuterVLAN != 0 {
		frame = append(frame, 0x81, 0x00)
		vlanTag := make([]byte, 2)
		binary.BigEndian.PutUint16(vlanTag, req.OuterVLAN)
		frame = append(frame, vlanTag...)

		if req.InnerVLAN != 0 {
			frame = append(frame, 0x81, 0x00)
			vlanTag := make([]byte, 2)
			binary.BigEndian.PutUint16(vlanTag, req.InnerVLAN)
			frame = append(frame, vlanTag...)
		}
	}

	frame = append(frame, byte(ethernet.EtherTypeARP>>8), byte(ethernet.EtherTypeARP&0xFF))

	arpReply := make([]byte, 28)
	binary.BigEndian.PutUint16(arpReply[0:2], 1)
	binary.BigEndian.PutUint16(arpReply[2:4], ethernet.EtherTypeIPv4)
	arpReply[4] = 6
	arpReply[5] = 4
	binary.BigEndian.PutUint16(arpReply[6:8], 2)
	copy(arpReply[8:14], m.gatewayMAC)
	copy(arpReply[14:18], req.TargetIP.To4())
	copy(arpReply[18:24], req.SenderMAC)
	copy(arpReply[24:28], req.SenderIP.To4())

	frame = append(frame, arpReply...)

	return frame
}

func (m *MemifHandler) SendPacket(pkt *dataplane.EgressPacket) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.connected || m.txQueue == nil {
		return fmt.Errorf("memif not connected")
	}

	frame, err := m.buildEgressFrame(pkt)
	if err != nil {
		return fmt.Errorf("build egress frame: %w", err)
	}

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

func (m *MemifHandler) buildEgressFrame(pkt *dataplane.EgressPacket) ([]byte, error) {
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true}

	finalEtherType := layers.EthernetType(pkt.EtherType)
	if finalEtherType == 0 {
		finalEtherType = layers.EthernetTypeIPv4
	}

	eth := &layers.Ethernet{
		SrcMAC: pkt.SrcMAC,
		DstMAC: pkt.DstMAC,
	}

	var layerStack []gopacket.SerializableLayer

	if pkt.OuterVLAN > 0 && pkt.InnerVLAN > 0 {
		// QinQ: Ethernet -> Outer VLAN -> Inner VLAN -> Payload
		eth.EthernetType = layers.EthernetTypeDot1Q
		dot1qOuter := &layers.Dot1Q{
			VLANIdentifier: pkt.OuterVLAN,
			Type:           layers.EthernetTypeDot1Q,
		}
		dot1qInner := &layers.Dot1Q{
			VLANIdentifier: pkt.InnerVLAN,
			Type:           finalEtherType,
		}
		layerStack = []gopacket.SerializableLayer{eth, dot1qOuter, dot1qInner, gopacket.Payload(pkt.Payload)}
	} else if pkt.OuterVLAN > 0 {
		// Single VLAN: Ethernet -> VLAN -> Payload
		eth.EthernetType = layers.EthernetTypeDot1Q
		dot1q := &layers.Dot1Q{
			VLANIdentifier: pkt.OuterVLAN,
			Type:           finalEtherType,
		}
		layerStack = []gopacket.SerializableLayer{eth, dot1q, gopacket.Payload(pkt.Payload)}
	} else {
		// No VLAN: Ethernet -> Payload
		eth.EthernetType = finalEtherType
		layerStack = []gopacket.SerializableLayer{eth, gopacket.Payload(pkt.Payload)}
	}

	if err := gopacket.SerializeLayers(buf, opts, layerStack...); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (m *MemifHandler) Close() error {
	m.cancel()
	m.wg.Wait()

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
