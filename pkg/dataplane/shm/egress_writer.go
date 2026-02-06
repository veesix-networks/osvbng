package shm

import (
	"errors"
)

var (
	ErrRingFull     = errors.New("egress ring full")
	ErrPacketTooBig = errors.New("packet too big for slot")
)

type EgressWriter struct {
	client    *Client
	head      uint64
	mask      uint64
	slotIndex uint32
	slotCount uint32
}

func NewEgressWriter(client *Client) *EgressWriter {
	return &EgressWriter{
		client:    client,
		head:      client.egressRing.LoadHead(),
		mask:      RingMask(client.header.EgressRingSize),
		slotIndex: 0,
		slotCount: client.header.EgressDataSlots,
	}
}

func (w *EgressWriter) Write(swIfIndex uint32, data []byte) error {
	if uint32(len(data)) > w.client.header.SlotSize {
		return ErrPacketTooBig
	}

	tail := w.client.egressRing.LoadTail()
	if w.head-tail >= uint64(w.client.header.EgressRingSize) {
		return ErrRingFull
	}

	slot := w.client.GetEgressSlot(w.slotIndex)
	copy(slot, data)

	desc := &w.client.egressDescs[w.head&w.mask]
	desc.DataOffset = w.client.egressDataOffset + w.slotIndex*w.client.header.SlotSize
	desc.SwIfIndex = swIfIndex
	desc.DataLength = uint16(len(data))

	w.head++
	w.slotIndex = (w.slotIndex + 1) % w.slotCount

	return nil
}

func (w *EgressWriter) WriteBatch(packets []EgressPacket) (int, error) {
	tail := w.client.egressRing.LoadTail()
	freeSlots := uint64(w.client.header.EgressRingSize) - (w.head - tail)

	count := len(packets)
	if uint64(count) > freeSlots {
		count = int(freeSlots)
	}

	for i := 0; i < count; i++ {
		pkt := &packets[i]
		if uint32(len(pkt.Data)) > w.client.header.SlotSize {
			continue
		}

		slot := w.client.GetEgressSlot(w.slotIndex)
		copy(slot, pkt.Data)

		desc := &w.client.egressDescs[w.head&w.mask]
		desc.DataOffset = w.client.egressDataOffset + w.slotIndex*w.client.header.SlotSize
		desc.SwIfIndex = pkt.SwIfIndex
		desc.DataLength = uint16(len(pkt.Data))

		w.head++
		w.slotIndex = (w.slotIndex + 1) % w.slotCount
	}

	return count, nil
}

func (w *EgressWriter) Flush() error {
	w.client.egressRing.StoreHead(w.head)

	if w.client.egressRing.CompareAndSwapInterruptPending(0, 1) {
		return w.client.SignalEgress()
	}
	return nil
}

func (w *EgressWriter) Available() uint64 {
	tail := w.client.egressRing.LoadTail()
	return uint64(w.client.header.EgressRingSize) - (w.head - tail)
}

type EgressPacket struct {
	SwIfIndex uint32
	Data      []byte
}
