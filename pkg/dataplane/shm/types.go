package shm

import (
	"sync/atomic"
	"unsafe"
)

const (
	ShmMagic = 0x4F53564E47424E47
	// Version 2: one punt ring per VPP thread (was a single ring shared
	// by every worker, a publish race). The egress ring stays single.
	// This reader refuses a v1 region rather than misread its header.
	ShmVersion = 2

	DefaultPuntRingSize   = 4096
	DefaultEgressRingSize = 4096
	DefaultDataSlots      = 8192
	DefaultSlotSize       = 2048

	ShmPath         = "/dev/shm/osvbng-dataplane"
	EventfdSockPath = "/run/vpp/osvbng-punt.evt"
)

type Protocol uint8

const (
	ProtoDHCPv4      Protocol = 0
	ProtoDHCPv6      Protocol = 1
	ProtoARP         Protocol = 2
	ProtoPPPoEDisc   Protocol = 3
	ProtoPPPoESess   Protocol = 4
	ProtoIPv6ND      Protocol = 5
	ProtoL2TP        Protocol = 6
	ProtoL2GWTrigger Protocol = 7
	ProtoCount       Protocol = 8
)

// ShmHeader mirrors osvbng_shm_header_t (v2) byte for byte. One punt
// ring per VPP thread laid out at PuntRingOffset with PuntRingStride
// between them; a single egress ring; punt data slots for every ring
// then the egress data slots.
type ShmHeader struct {
	Magic            uint64
	Version          uint32
	Flags            uint32
	NPuntRings       uint32
	PuntRingSize     uint32
	PuntRingStride   uint32
	PuntRingOffset   uint32
	EgressRingOffset uint32
	EgressRingSize   uint32
	PuntDataOffset   uint32
	EgressDataOffset uint32
	SlotSize         uint32
	Reserved         [12]byte
}

type RingHeader struct {
	Head             uint64
	Tail             uint64
	InterruptPending uint8
	Reserved         [47]byte
}

type PuntDesc struct {
	DataOffset uint32
	SwIfIndex  uint32
	DataLength uint16
	Protocol   uint8
	Flags      uint8
	OuterVLAN  uint16
	InnerVLAN  uint16
	Timestamp  uint64
}

type EgressDesc struct {
	DataOffset uint32
	SwIfIndex  uint32
	DataLength uint16
	Reserved   uint16
}

var (
	_ [64]byte = [unsafe.Sizeof(ShmHeader{})]byte{}
	_ [64]byte = [unsafe.Sizeof(RingHeader{})]byte{}
	_ [24]byte = [unsafe.Sizeof(PuntDesc{})]byte{}
	_ [12]byte = [unsafe.Sizeof(EgressDesc{})]byte{}
)

type AtomicRingHeader struct {
	ring *RingHeader
}

func NewAtomicRingHeader(ring *RingHeader) *AtomicRingHeader {
	return &AtomicRingHeader{ring: ring}
}

func (a *AtomicRingHeader) LoadHead() uint64 {
	return atomic.LoadUint64(&a.ring.Head)
}

func (a *AtomicRingHeader) LoadTail() uint64 {
	return atomic.LoadUint64(&a.ring.Tail)
}

func (a *AtomicRingHeader) StoreHead(val uint64) {
	atomic.StoreUint64(&a.ring.Head, val)
}

func (a *AtomicRingHeader) StoreTail(val uint64) {
	atomic.StoreUint64(&a.ring.Tail, val)
}

func (a *AtomicRingHeader) LoadInterruptPending() uint8 {
	return uint8(atomic.LoadUint32((*uint32)(unsafe.Pointer(&a.ring.InterruptPending))))
}

func (a *AtomicRingHeader) StoreInterruptPending(val uint8) {
	atomic.StoreUint32((*uint32)(unsafe.Pointer(&a.ring.InterruptPending)), uint32(val))
}

func (a *AtomicRingHeader) CompareAndSwapInterruptPending(old, new uint8) bool {
	return atomic.CompareAndSwapUint32(
		(*uint32)(unsafe.Pointer(&a.ring.InterruptPending)),
		uint32(old),
		uint32(new),
	)
}

func (a *AtomicRingHeader) RingCount() uint64 {
	head := a.LoadHead()
	tail := a.LoadTail()
	return head - tail
}

func (a *AtomicRingHeader) RingHasSpace(ringSize uint32, n uint32) bool {
	head := a.LoadHead()
	tail := a.LoadTail()
	return (head - tail + uint64(n)) <= uint64(ringSize)
}

func (a *AtomicRingHeader) RingHasData(n uint32) bool {
	head := a.LoadHead()
	tail := a.LoadTail()
	return (head - tail) >= uint64(n)
}

func RingMask(size uint32) uint64 {
	return uint64(size - 1)
}
