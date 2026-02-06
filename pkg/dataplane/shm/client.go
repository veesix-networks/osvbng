package shm

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

type Client struct {
	shmFd   int
	shmData []byte
	shmSize int

	puntEventfd   int
	egressEventfd int

	header      *ShmHeader
	puntRing    *AtomicRingHeader
	puntDescs   []PuntDesc
	egressRing  *AtomicRingHeader
	egressDescs []EgressDesc

	puntDataOffset   uint32
	egressDataOffset uint32
}

func NewClient() *Client {
	return &Client{
		shmFd:         -1,
		puntEventfd:   -1,
		egressEventfd: -1,
	}
}

func (c *Client) Connect() error {
	if err := c.openShm(); err != nil {
		return fmt.Errorf("open shm: %w", err)
	}

	if err := c.receiveEventfds(); err != nil {
		c.Close()
		return fmt.Errorf("receive eventfds: %w", err)
	}

	if err := c.mapStructures(); err != nil {
		c.Close()
		return fmt.Errorf("map structures: %w", err)
	}

	return nil
}

func (c *Client) openShm() error {
	fd, err := unix.Open(ShmPath, unix.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open %s: %w", ShmPath, err)
	}
	c.shmFd = fd

	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return fmt.Errorf("fstat: %w", err)
	}
	c.shmSize = int(stat.Size)

	data, err := unix.Mmap(fd, 0, c.shmSize, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		return fmt.Errorf("mmap: %w", err)
	}
	c.shmData = data

	return nil
}

func (c *Client) receiveEventfds() error {
	fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		return fmt.Errorf("socket: %w", err)
	}
	defer unix.Close(fd)

	addr := &unix.SockaddrUnix{Name: EventfdSockPath}
	if err := unix.Connect(fd, addr); err != nil {
		return fmt.Errorf("connect %s: %w", EventfdSockPath, err)
	}

	buf := make([]byte, 1)
	oob := make([]byte, 64)

	n, oobn, _, _, err := unix.Recvmsg(fd, buf, oob, 0)
	if err != nil {
		return fmt.Errorf("recvmsg: %w", err)
	}
	if n != 1 || buf[0] != 'O' {
		return fmt.Errorf("unexpected message: n=%d buf=%v", n, buf)
	}

	msgs, err := unix.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		return fmt.Errorf("parse control message: %w", err)
	}
	if len(msgs) != 1 {
		return fmt.Errorf("expected 1 control message, got %d", len(msgs))
	}

	fds, err := unix.ParseUnixRights(&msgs[0])
	if err != nil {
		return fmt.Errorf("parse unix rights: %w", err)
	}
	if len(fds) != 2 {
		return fmt.Errorf("expected 2 fds, got %d", len(fds))
	}

	c.puntEventfd = fds[0]
	c.egressEventfd = fds[1]

	return nil
}

func (c *Client) mapStructures() error {
	if c.shmSize < int(unsafe.Sizeof(ShmHeader{})) {
		return fmt.Errorf("shm too small: %d", c.shmSize)
	}

	c.header = (*ShmHeader)(unsafe.Pointer(&c.shmData[0]))

	if c.header.Magic != ShmMagic {
		return fmt.Errorf("bad magic: %x", c.header.Magic)
	}
	if c.header.Version != ShmVersion {
		return fmt.Errorf("version mismatch: %d", c.header.Version)
	}

	puntRingHdr := (*RingHeader)(unsafe.Pointer(&c.shmData[c.header.PuntRingOffset]))
	c.puntRing = NewAtomicRingHeader(puntRingHdr)

	puntDescsOffset := c.header.PuntRingOffset + uint32(unsafe.Sizeof(RingHeader{}))
	c.puntDescs = unsafe.Slice(
		(*PuntDesc)(unsafe.Pointer(&c.shmData[puntDescsOffset])),
		c.header.PuntRingSize,
	)

	egressRingHdr := (*RingHeader)(unsafe.Pointer(&c.shmData[c.header.EgressRingOffset]))
	c.egressRing = NewAtomicRingHeader(egressRingHdr)

	egressDescsOffset := c.header.EgressRingOffset + uint32(unsafe.Sizeof(RingHeader{}))
	c.egressDescs = unsafe.Slice(
		(*EgressDesc)(unsafe.Pointer(&c.shmData[egressDescsOffset])),
		c.header.EgressRingSize,
	)

	c.puntDataOffset = c.header.DataRegionOffset
	c.egressDataOffset = c.header.DataRegionOffset + c.header.PuntDataSlots*c.header.SlotSize

	return nil
}

func (c *Client) Close() error {
	var errs []error

	if c.shmData != nil {
		if err := unix.Munmap(c.shmData); err != nil {
			errs = append(errs, err)
		}
		c.shmData = nil
	}

	if c.shmFd >= 0 {
		unix.Close(c.shmFd)
		c.shmFd = -1
	}

	if c.puntEventfd >= 0 {
		unix.Close(c.puntEventfd)
		c.puntEventfd = -1
	}

	if c.egressEventfd >= 0 {
		unix.Close(c.egressEventfd)
		c.egressEventfd = -1
	}

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

func (c *Client) PuntEventfd() int {
	return c.puntEventfd
}

func (c *Client) EgressEventfd() int {
	return c.egressEventfd
}

func (c *Client) Header() *ShmHeader {
	return c.header
}

func (c *Client) GetPuntData(offset uint32, length uint16) []byte {
	return c.shmData[offset : offset+uint32(length)]
}

func (c *Client) GetEgressSlot(index uint32) []byte {
	offset := c.egressDataOffset + index*c.header.SlotSize
	return c.shmData[offset : offset+c.header.SlotSize]
}

func (c *Client) PuntFd() *os.File {
	return os.NewFile(uintptr(c.puntEventfd), "punt-eventfd")
}

func (c *Client) EgressFd() *os.File {
	return os.NewFile(uintptr(c.egressEventfd), "egress-eventfd")
}

func (c *Client) WaitPunt() error {
	var buf [8]byte
	_, err := syscall.Read(c.puntEventfd, buf[:])
	return err
}

func (c *Client) SignalEgress() error {
	var buf [8]byte
	buf[0] = 1
	_, err := syscall.Write(c.egressEventfd, buf[:])
	return err
}
