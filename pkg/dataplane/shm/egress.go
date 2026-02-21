package shm

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/veesix-networks/osvbng/pkg/dataplane"
	"github.com/veesix-networks/osvbng/pkg/ethernet"
	"github.com/veesix-networks/osvbng/pkg/logger"
)

type Egress struct {
	client    *Client
	writer    *EgressWriter
	logger    *slog.Logger
	mu        sync.Mutex
	txChan    chan *dataplane.EgressPacket
	closeChan chan struct{}
	wg        sync.WaitGroup
}

func NewEgress(client *Client) *Egress {
	e := &Egress{
		client:    client,
		writer:    NewEgressWriter(client),
		logger:    logger.Get(logger.Dataplane),
		txChan:    make(chan *dataplane.EgressPacket, 1000),
		closeChan: make(chan struct{}),
	}

	e.wg.Add(1)
	go e.txFlushLoop()

	return e
}

func (e *Egress) txFlushLoop() {
	defer e.wg.Done()

	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()

	pending := 0

	for {
		select {
		case <-e.closeChan:
			if pending > 0 {
				e.flush()
			}
			return
		case pkt := <-e.txChan:
			if err := e.writePacket(pkt); err != nil {
				e.logger.Error("Failed to write egress packet", "error", err)
				continue
			}
			pending++
			if pending >= 64 {
				e.flush()
				pending = 0
			}
		case <-ticker.C:
			if pending > 0 {
				e.flush()
				pending = 0
			}
		}
	}
}

func (e *Egress) writePacket(pkt *dataplane.EgressPacket) error {
	frame := e.buildFrame(pkt)

	e.mu.Lock()
	defer e.mu.Unlock()

	return e.writer.Write(pkt.SwIfIndex, frame)
}

func (e *Egress) flush() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.writer.Flush(); err != nil {
		e.logger.Error("Failed to flush egress", "error", err)
	}
}

func (e *Egress) buildFrame(pkt *dataplane.EgressPacket) []byte {
	frame := make([]byte, 0, 14+8+len(pkt.Payload))

	frame = append(frame, pkt.DstMAC...)
	frame = append(frame, pkt.SrcMAC...)

	etherType := pkt.EtherType
	if etherType == 0 {
		etherType = ethernet.EtherTypeIPv4
	}

	if pkt.OuterVLAN > 0 && pkt.InnerVLAN > 0 {
		outerTPID := pkt.OuterTPID
		if outerTPID == 0 {
			outerTPID = 0x88A8
		}
		tpid := make([]byte, 2)
		binary.BigEndian.PutUint16(tpid, outerTPID)
		frame = append(frame, tpid...)
		vlan := make([]byte, 2)
		binary.BigEndian.PutUint16(vlan, pkt.OuterVLAN)
		frame = append(frame, vlan...)

		frame = append(frame, 0x81, 0x00)
		binary.BigEndian.PutUint16(vlan, pkt.InnerVLAN)
		frame = append(frame, vlan...)
	} else if pkt.OuterVLAN > 0 {
		frame = append(frame, 0x81, 0x00)
		vlan := make([]byte, 2)
		binary.BigEndian.PutUint16(vlan, pkt.OuterVLAN)
		frame = append(frame, vlan...)
	}

	et := make([]byte, 2)
	binary.BigEndian.PutUint16(et, etherType)
	frame = append(frame, et...)

	frame = append(frame, pkt.Payload...)

	return frame
}

func (e *Egress) SendPacket(pkt *dataplane.EgressPacket) error {
	select {
	case e.txChan <- pkt:
		return nil
	default:
		return fmt.Errorf("tx channel full")
	}
}

func (e *Egress) Reconnect(client *Client) error {
	e.logger.Info("Reconnecting SHM egress")

	close(e.closeChan)
	e.wg.Wait()

	e.client = client
	e.writer = NewEgressWriter(client)
	e.txChan = make(chan *dataplane.EgressPacket, 1000)
	e.closeChan = make(chan struct{})

	e.wg.Add(1)
	go e.txFlushLoop()

	e.logger.Info("SHM egress reconnected")
	return nil
}

func (e *Egress) Close() error {
	close(e.closeChan)
	e.wg.Wait()
	return nil
}
