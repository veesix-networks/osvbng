package vpp

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/veesix-networks/osvbng/pkg/ifmgr"
	"github.com/veesix-networks/osvbng/pkg/logger"
	southbound "github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
	"go.fd.io/govpp/api"
	"go.fd.io/govpp/core"
)

var _ southbound.Southbound = (*VPP)(nil)

type VPP struct {
	conn           *core.Connection
	ifMgr          *ifmgr.Manager
	logger         *slog.Logger
	fibChan        api.Channel
	fibMux         sync.Mutex
	useDPDK        bool
	asyncWorker    *AsyncWorker
	statsClient    *StatsClient
	vrfResolver    func(string) (uint32, bool, bool, error)
	lcpNs          *netlink.Handle
	policerNames map[uint32][2]string
	policerMu    sync.Mutex
}

type VPPConfig struct {
	Connection      *core.Connection
	IfMgr           *ifmgr.Manager
	UseDPDK         bool
	StatsSocketPath string
}

func NewVPP(cfg VPPConfig) (*VPP, error) {
	if cfg.Connection == nil {
		return nil, fmt.Errorf("VPP connection is required")
	}
	if cfg.IfMgr == nil {
		return nil, fmt.Errorf("IfMgr is required")
	}

	conn := cfg.Connection

	fibChan, err := conn.NewAPIChannel()
	if err != nil {
		return nil, fmt.Errorf("create FIB API channel: %w", err)
	}

	asyncWorker, err := NewAsyncWorker(conn, DefaultAsyncWorkerConfig())
	if err != nil {
		fibChan.Close()
		return nil, fmt.Errorf("create async worker: %w", err)
	}

	statsClient := NewStatsClient(cfg.StatsSocketPath)
	if err := statsClient.Connect(); err != nil {
		fibChan.Close()
		return nil, fmt.Errorf("connect to stats: %w", err)
	}

	v := &VPP{
		conn:           conn,
		ifMgr:          cfg.IfMgr,
		logger:         logger.Get(logger.Southbound),
		fibChan:        fibChan,
		useDPDK:        cfg.UseDPDK,
		asyncWorker:    asyncWorker,
		statsClient:    statsClient,
		policerNames: make(map[uint32][2]string),
	}

	if err := v.LoadInterfaces(); err != nil {
		statsClient.Disconnect()
		fibChan.Close()
		return nil, fmt.Errorf("load interfaces: %w", err)
	}

	if err := v.LoadIPState(); err != nil {
		v.logger.Warn("Failed to load IP state at startup", "error", err)
	}

	asyncWorker.Start()

	v.logger.Debug("Connected to VPP", "interfaces_loaded", len(v.ifMgr.List()))

	return v, nil
}

func (v *VPP) SetVRFResolver(resolver func(string) (uint32, bool, bool, error)) {
	v.vrfResolver = resolver
}

func (v *VPP) SetLCPNetNs(nsName string) error {
	nsHandle, err := netns.GetFromName(nsName)
	if err != nil {
		return fmt.Errorf("get netns %q: %w", nsName, err)
	}

	h, err := netlink.NewHandleAt(nsHandle)
	if err != nil {
		nsHandle.Close()
		return fmt.Errorf("create netlink handle for netns %q: %w", nsName, err)
	}

	lo, err := h.LinkByName("lo")
	if err == nil {
		if err := h.LinkSetUp(lo); err != nil {
			v.logger.Warn("Failed to bring up loopback in LCP namespace", "netns", nsName, "error", err)
		}
	}

	v.lcpNs = h
	v.logger.Info("LCP namespace configured", "netns", nsName)
	return nil
}

func (v *VPP) Close() error {
	if v.asyncWorker != nil {
		v.asyncWorker.Stop()
	}
	if v.lcpNs != nil {
		v.lcpNs.Close()
	}
	if v.statsClient != nil {
		v.statsClient.Disconnect()
	}
	if v.fibChan != nil {
		v.fibChan.Close()
	}
	v.conn.Disconnect()
	return nil
}
