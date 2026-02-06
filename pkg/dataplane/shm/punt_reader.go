package shm

type PuntPacket struct {
	SwIfIndex uint32
	Protocol  Protocol
	OuterVLAN uint16
	InnerVLAN uint16
	Timestamp uint64
	Data      []byte
}

type PuntReader struct {
	client *Client
	tail   uint64
	mask   uint64
}

func NewPuntReader(client *Client) *PuntReader {
	return &PuntReader{
		client: client,
		tail:   client.puntRing.LoadTail(),
		mask:   RingMask(client.header.PuntRingSize),
	}
}

func (r *PuntReader) Available() uint64 {
	head := r.client.puntRing.LoadHead()
	return head - r.tail
}

func (r *PuntReader) Read() (*PuntPacket, bool) {
	head := r.client.puntRing.LoadHead()
	if r.tail == head {
		return nil, false
	}

	desc := &r.client.puntDescs[r.tail&r.mask]

	pkt := &PuntPacket{
		SwIfIndex: desc.SwIfIndex,
		Protocol:  Protocol(desc.Protocol),
		OuterVLAN: desc.OuterVLAN,
		InnerVLAN: desc.InnerVLAN,
		Timestamp: desc.Timestamp,
		Data:      make([]byte, desc.DataLength),
	}
	copy(pkt.Data, r.client.GetPuntData(desc.DataOffset, desc.DataLength))

	r.tail++

	return pkt, true
}

func (r *PuntReader) ReadBatch(max int) []*PuntPacket {
	head := r.client.puntRing.LoadHead()
	available := head - r.tail
	if available == 0 {
		return nil
	}

	count := int(available)
	if count > max {
		count = max
	}

	packets := make([]*PuntPacket, count)
	for i := 0; i < count; i++ {
		desc := &r.client.puntDescs[r.tail&r.mask]

		packets[i] = &PuntPacket{
			SwIfIndex: desc.SwIfIndex,
			Protocol:  Protocol(desc.Protocol),
			OuterVLAN: desc.OuterVLAN,
			InnerVLAN: desc.InnerVLAN,
			Timestamp: desc.Timestamp,
			Data:      make([]byte, desc.DataLength),
		}
		copy(packets[i].Data, r.client.GetPuntData(desc.DataOffset, desc.DataLength))

		r.tail++
	}

	return packets
}

func (r *PuntReader) Commit() {
	r.client.puntRing.StoreTail(r.tail)
	r.client.puntRing.StoreInterruptPending(0)
}

func (r *PuntReader) Wait() error {
	return r.client.WaitPunt()
}
