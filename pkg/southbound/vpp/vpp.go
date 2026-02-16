package vpp

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/veesix-networks/osvbng/pkg/ifmgr"
	"github.com/veesix-networks/osvbng/pkg/logger"
	southbound "github.com/veesix-networks/osvbng/pkg/southbound"
	"go.fd.io/govpp/api"
	"go.fd.io/govpp/core"
)

var _ southbound.Southbound = (*VPP)(nil)

type VPP struct {
	conn        *core.Connection
	ifMgr       *ifmgr.Manager
	logger      *slog.Logger
	fibChan     api.Channel
	fibMux      sync.Mutex
	useDPDK     bool
	asyncWorker *AsyncWorker
	statsClient *StatsClient
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
		conn:        conn,
		ifMgr:       cfg.IfMgr,
		logger:      logger.Get(logger.Southbound),
		fibChan:     fibChan,
		useDPDK:     cfg.UseDPDK,
		asyncWorker: asyncWorker,
		statsClient: statsClient,
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

func (v *VPP) Close() error {
	if v.asyncWorker != nil {
		v.asyncWorker.Stop()
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
