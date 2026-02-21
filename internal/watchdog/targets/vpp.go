package targets

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/veesix-networks/osvbng/internal/watchdog"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

const (
	DefaultVPPBinary      = "/usr/bin/vpp"
	DefaultVPPConfigPath  = "/etc/osvbng/dataplane.conf"
	DefaultVPPStderrLog   = "/var/log/osvbng/dataplane-stderr.log"
	DefaultVPPServiceName = "vpp"
)

type VPPCallbacks struct {
	OnDown    func()
	OnUp      func()
	OnRecover func(ctx context.Context) error
}

type VPPTarget struct {
	sb         southbound.Southbound
	apiSocket  string
	vppBinary  string
	configPath string
	callbacks  VPPCallbacks
	critical   bool
	useSystemd bool
}

func NewVPPTarget(sb southbound.Southbound, apiSocket string, critical bool) *VPPTarget {
	if apiSocket == "" {
		apiSocket = "/run/osvbng/dataplane_api.sock"
	}
	return &VPPTarget{
		sb:         sb,
		apiSocket:  apiSocket,
		vppBinary:  DefaultVPPBinary,
		configPath: DefaultVPPConfigPath,
		critical:   critical,
		useSystemd: vppManagedBySystemd(),
	}
}

func (t *VPPTarget) SetCallbacks(cb VPPCallbacks) {
	t.callbacks = cb
}

func (t *VPPTarget) Name() string { return "vpp" }

func (t *VPPTarget) Check(ctx context.Context) *watchdog.HealthResult {
	start := time.Now()

	type versionResult struct {
		err error
	}

	ch := make(chan versionResult, 1)
	go func() {
		_, err := t.sb.GetVersion(ctx)
		ch <- versionResult{err}
	}()

	select {
	case <-ctx.Done():
		return watchdog.NewHealthResult(false, ctx.Err(), time.Since(start))
	case r := <-ch:
		return watchdog.NewHealthResult(r.err == nil, r.err, time.Since(start))
	}
}

func (t *VPPTarget) Connect(ctx context.Context) error {
	conn, err := net.DialTimeout("unix", t.apiSocket, 3*time.Second)
	if err != nil {
		return fmt.Errorf("VPP API socket not available: %w", err)
	}
	conn.Close()
	return nil
}

func (t *VPPTarget) Disconnect() error {
	return nil
}

func (t *VPPTarget) Restart(ctx context.Context) error {
	if t.useSystemd {
		return t.restartSystemd(ctx)
	}
	return t.restartDirect(ctx)
}

func (t *VPPTarget) restartSystemd(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "systemctl", "restart", DefaultVPPServiceName)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl restart vpp: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (t *VPPTarget) restartDirect(ctx context.Context) error {
	if pids, err := findVPPPids(); err == nil && len(pids) > 0 {
		for _, pid := range pids {
			_ = exec.Command("kill", "-9", strconv.Itoa(pid)).Run()
		}
		time.Sleep(500 * time.Millisecond)
	}

	stderr, err := os.OpenFile(DefaultVPPStderrLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open stderr log: %w", err)
	}

	cmd := exec.Command(t.vppBinary, "-c", t.configPath)
	cmd.Stdout = stderr
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		stderr.Close()
		return fmt.Errorf("start VPP: %w", err)
	}

	go func() {
		cmd.Wait()
		stderr.Close()
	}()

	return nil
}

func (t *VPPTarget) OnDown() {
	if t.callbacks.OnDown != nil {
		t.callbacks.OnDown()
	}
}

func (t *VPPTarget) OnUp() {
	if t.callbacks.OnUp != nil {
		t.callbacks.OnUp()
	}
}

func (t *VPPTarget) Recover(ctx context.Context) error {
	if t.callbacks.OnRecover != nil {
		return t.callbacks.OnRecover(ctx)
	}
	return nil
}

func (t *VPPTarget) Critical() bool { return t.critical }

func vppManagedBySystemd() bool {
	err := exec.Command("systemctl", "cat", DefaultVPPServiceName).Run()
	return err == nil
}

func findVPPPids() ([]int, error) {
	out, err := exec.Command("pidof", "vpp").Output()
	if err != nil {
		return nil, err
	}
	var pids []int
	for _, s := range strings.Fields(string(out)) {
		if pid, err := strconv.Atoi(s); err == nil {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}
