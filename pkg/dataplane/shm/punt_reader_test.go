package shm

import (
	"testing"
	"unsafe"
)

// buildRegion lays out a v2 shm region by hand using the same offset
// math the VPP plugin uses, so the test exercises the real header
// parsing and per-ring pointer setup rather than a mock.
func buildRegion(t *testing.T, nRings, ringSize, egressRingSize, slotSize uint32) *Client {
	t.Helper()

	ringHdrSize := uint32(unsafe.Sizeof(RingHeader{}))
	puntDescSize := uint32(unsafe.Sizeof(PuntDesc{}))
	egressDescSize := uint32(unsafe.Sizeof(EgressDesc{}))

	stride := ringHdrSize + ringSize*puntDescSize

	puntRingOffset := uint32(unsafe.Sizeof(ShmHeader{}))
	egressRingOffset := puntRingOffset + nRings*stride
	puntDataOffset := egressRingOffset + ringHdrSize + egressRingSize*egressDescSize
	egressDataOffset := puntDataOffset + nRings*ringSize*slotSize
	total := egressDataOffset + egressRingSize*slotSize

	data := make([]byte, total)
	c := &Client{shmData: data, shmSize: len(data)}

	hdr := (*ShmHeader)(unsafe.Pointer(&data[0]))
	hdr.Magic = ShmMagic
	hdr.Version = ShmVersion
	hdr.NPuntRings = nRings
	hdr.PuntRingSize = ringSize
	hdr.PuntRingStride = stride
	hdr.PuntRingOffset = puntRingOffset
	hdr.EgressRingOffset = egressRingOffset
	hdr.EgressRingSize = egressRingSize
	hdr.PuntDataOffset = puntDataOffset
	hdr.EgressDataOffset = egressDataOffset
	hdr.SlotSize = slotSize

	if err := c.mapStructures(); err != nil {
		t.Fatalf("mapStructures: %v", err)
	}
	return c
}

// publish writes one descriptor + its payload into ring `ring` at the
// producer head and bumps the head, mimicking osvbng_punt_to_shm.
func publish(c *Client, ring uint32, proto Protocol, swIfIndex uint32, payload []byte) {
	head := c.puntRings[ring].LoadHead()
	slot := head & RingMask(c.header.PuntRingSize)

	dataOff := c.header.PuntDataOffset +
		(ring*c.header.PuntRingSize+uint32(slot))*c.header.SlotSize
	copy(c.shmData[dataOff:dataOff+uint32(len(payload))], payload)

	desc := &c.puntDescs[ring][slot]
	desc.DataOffset = dataOff
	desc.SwIfIndex = swIfIndex
	desc.DataLength = uint16(len(payload))
	desc.Protocol = uint8(proto)

	c.puntRings[ring].StoreHead(head + 1)
	c.puntRings[ring].StoreInterruptPending(1)
}

func TestShmHeaderIs64Bytes(t *testing.T) {
	if got := unsafe.Sizeof(ShmHeader{}); got != 64 {
		t.Fatalf("ShmHeader size = %d, want 64", got)
	}
}

func TestPuntReaderRefusesV1(t *testing.T) {
	c := buildRegion(t, 2, 4, 4, 64)
	c.header.Version = 1
	if err := c.mapStructures(); err == nil {
		t.Fatal("mapStructures accepted a v1 region")
	}
}

func TestPuntReaderDrainsAllRings(t *testing.T) {
	c := buildRegion(t, 3, 8, 8, 128)
	r := NewPuntReader(c)

	// Punt lands only on worker rings 1 and 2; ring 0 (main) stays empty,
	// the exact shape the live ARP smoke test produced.
	publish(c, 1, ProtoARP, 10, []byte("arp-a"))
	publish(c, 2, ProtoDHCPv4, 20, []byte("dhcp-b"))
	publish(c, 1, ProtoARP, 11, []byte("arp-c"))

	seen := map[string]uint32{}
	for {
		pkt, ok := r.Read()
		if !ok {
			break
		}
		seen[string(pkt.Data)] = pkt.SwIfIndex
	}

	if len(seen) != 3 {
		t.Fatalf("drained %d packets, want 3: %v", len(seen), seen)
	}
	if seen["arp-a"] != 10 || seen["arp-c"] != 11 || seen["dhcp-b"] != 20 {
		t.Fatalf("wrong sw_if_index mapping: %v", seen)
	}

	// Commit must advance every ring tail to its head and clear the
	// interrupt flag so the next 0->1 edge re-signals.
	r.Commit()
	for i, ring := range c.puntRings {
		if h, tl := ring.LoadHead(), ring.LoadTail(); h != tl {
			t.Fatalf("ring %d not drained: head=%d tail=%d", i, h, tl)
		}
		if ring.LoadInterruptPending() != 0 {
			t.Fatalf("ring %d interrupt not cleared", i)
		}
	}
}

func TestPuntReaderRoundRobinFairness(t *testing.T) {
	c := buildRegion(t, 2, 8, 8, 64)
	r := NewPuntReader(c)

	// Ring 0 is backlogged; ring 1 has one packet. Round-robin must not
	// let ring 0 starve ring 1.
	for i := 0; i < 4; i++ {
		publish(c, 0, ProtoARP, 100, []byte{byte('0'), byte('a' + i)})
	}
	publish(c, 1, ProtoARP, 200, []byte("1x"))

	first, ok := r.Read()
	if !ok {
		t.Fatal("expected a packet")
	}
	// cur starts at 0, so the first Read pulls from ring 0, then advances
	// cur to ring 1; the second Read must come from ring 1.
	second, ok := r.Read()
	if !ok {
		t.Fatal("expected a second packet")
	}
	if first.SwIfIndex != 100 || second.SwIfIndex != 200 {
		t.Fatalf("round-robin broken: first=%d second=%d", first.SwIfIndex, second.SwIfIndex)
	}
}

func TestPuntReaderBatchAcrossRings(t *testing.T) {
	c := buildRegion(t, 3, 8, 8, 64)
	r := NewPuntReader(c)

	publish(c, 0, ProtoARP, 1, []byte("a"))
	publish(c, 1, ProtoARP, 2, []byte("b"))
	publish(c, 2, ProtoARP, 3, []byte("c"))

	batch := r.ReadBatch(10)
	if len(batch) != 3 {
		t.Fatalf("batch drained %d, want 3", len(batch))
	}

	if got := r.ReadBatch(10); got != nil {
		t.Fatalf("expected empty batch after drain, got %d", len(got))
	}
}
