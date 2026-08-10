package shm

type PuntPacket struct {
	SwIfIndex uint32
	Protocol  Protocol
	OuterVLAN uint16
	InnerVLAN uint16
	Timestamp uint64
	Data      []byte
}

// PuntReader drains one punt ring per VPP thread. Each ring is SPSC
// (its owning worker is the only producer), so the reader keeps a tail
// per ring and clears each ring's interrupt flag on commit. `cur`
// round-robins the starting ring so no ring is starved under load.
type PuntReader struct {
	client *Client
	tails  []uint64
	mask   uint64
	cur    int
}

func NewPuntReader(client *Client) *PuntReader {
	tails := make([]uint64, len(client.puntRings))
	for i, ring := range client.puntRings {
		tails[i] = ring.LoadTail()
	}
	return &PuntReader{
		client: client,
		tails:  tails,
		mask:   RingMask(client.header.PuntRingSize),
	}
}

func (r *PuntReader) Available() uint64 {
	var total uint64
	for i, ring := range r.client.puntRings {
		total += ring.LoadHead() - r.tails[i]
	}
	return total
}

func (r *PuntReader) readDesc(ring int) *PuntPacket {
	desc := &r.client.puntDescs[ring][r.tails[ring]&r.mask]

	pkt := &PuntPacket{
		SwIfIndex: desc.SwIfIndex,
		Protocol:  Protocol(desc.Protocol),
		OuterVLAN: desc.OuterVLAN,
		InnerVLAN: desc.InnerVLAN,
		Timestamp: desc.Timestamp,
		Data:      make([]byte, desc.DataLength),
	}
	copy(pkt.Data, r.client.GetPuntData(desc.DataOffset, desc.DataLength))

	r.tails[ring]++
	return pkt
}

func (r *PuntReader) Read() (*PuntPacket, bool) {
	n := len(r.client.puntRings)
	for i := 0; i < n; i++ {
		ring := (r.cur + i) % n
		if r.tails[ring] == r.client.puntRings[ring].LoadHead() {
			continue
		}
		pkt := r.readDesc(ring)
		// Resume from the next ring so a busy ring can't monopolise.
		r.cur = (ring + 1) % n
		return pkt, true
	}
	return nil, false
}

func (r *PuntReader) ReadBatch(max int) []*PuntPacket {
	n := len(r.client.puntRings)
	packets := make([]*PuntPacket, 0, max)

	for i := 0; i < n && len(packets) < max; i++ {
		ring := (r.cur + i) % n
		head := r.client.puntRings[ring].LoadHead()
		for r.tails[ring] != head && len(packets) < max {
			packets = append(packets, r.readDesc(ring))
		}
	}

	if len(packets) == 0 {
		return nil
	}
	r.cur = (r.cur + 1) % n
	return packets
}

func (r *PuntReader) Commit() {
	for i, ring := range r.client.puntRings {
		ring.StoreTail(r.tails[i])
		ring.StoreInterruptPending(0)
	}
}

func (r *PuntReader) Wait() error {
	return r.client.WaitPunt()
}
