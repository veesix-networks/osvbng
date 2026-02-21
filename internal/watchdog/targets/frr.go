package targets

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/veesix-networks/osvbng/internal/watchdog"
)

const defaultVTYSocket = "/var/run/frr/watchfrr.vty"

type FRRTarget struct {
	socketPath string
	conn       net.Conn
	critical   bool
}

func NewFRRTarget(socketPath string, critical bool) *FRRTarget {
	if socketPath == "" {
		socketPath = defaultVTYSocket
	}
	return &FRRTarget{
		socketPath: socketPath,
		critical:   critical,
	}
}

func (t *FRRTarget) Name() string { return "frr" }

func (t *FRRTarget) Check(ctx context.Context) *watchdog.HealthResult {
	start := time.Now()

	conn, err := net.DialTimeout("unix", t.socketPath, 2*time.Second)
	if err != nil {
		return watchdog.NewHealthResult(false, fmt.Errorf("dial vty socket: %w", err), time.Since(start))
	}
	conn.Close()

	return watchdog.NewHealthResult(true, nil, time.Since(start))
}

func (t *FRRTarget) Connect(ctx context.Context) error {
	conn, err := net.DialTimeout("unix", t.socketPath, 2*time.Second)
	if err != nil {
		return fmt.Errorf("connect to FRR vty: %w", err)
	}
	t.conn = conn
	return nil
}

func (t *FRRTarget) Disconnect() error {
	if t.conn != nil {
		err := t.conn.Close()
		t.conn = nil
		return err
	}
	return nil
}

func (t *FRRTarget) Restart(ctx context.Context) error {
	return fmt.Errorf("FRR restart not supported (managed by watchfrr)")
}

func (t *FRRTarget) OnDown() {}
func (t *FRRTarget) OnUp()   {}

func (t *FRRTarget) Recover(ctx context.Context) error {
	return nil
}

func (t *FRRTarget) Critical() bool { return t.critical }
