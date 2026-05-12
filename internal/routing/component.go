package routing

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models/protocols/bgp"
	"github.com/veesix-networks/osvbng/pkg/models/vrf"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

type Component struct {
	*component.Base
	logger     *logger.Logger
	southbound southbound.Southbound
	configMgr  component.ConfigManager
}

func New(deps component.Dependencies) (*Component, error) {
	log := logger.Get(logger.Routing)

	c := &Component{
		Base:       component.NewBase("routing"),
		logger:     log,
		southbound: deps.Southbound,
		configMgr:  deps.ConfigManager,
	}

	return c, nil
}

func (c *Component) Start(ctx context.Context) error {
	c.StartContext(ctx)
	c.logger.Info("Starting routing component")
	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	c.logger.Info("Stopping routing component")
	c.StopContext()
	return nil
}

func (c *Component) ConfigureBGP(asn uint32, routerID string) error {
	args := []string{"-c", "configure terminal", "-c", fmt.Sprintf("router bgp %d", asn)}
	if routerID != "" {
		args = append(args, "-c", fmt.Sprintf("bgp router-id %s", routerID))
	}

	_, err := c.execVtysh(args...)
	return err
}

func (c *Component) RemoveBGP(asn uint32) error {
	_, err := c.execVtysh("-c", "configure terminal", "-c", fmt.Sprintf("no router bgp %d", asn))
	return err
}

func (c *Component) AdvertiseBGPNetwork(asn uint32, vrf string, prefix string, ipv6 bool) error {
	af := "ipv4"
	routeCmd := fmt.Sprintf("ip route %s Null0", prefix)
	if ipv6 {
		af = "ipv6"
		routeCmd = fmt.Sprintf("ipv6 route %s Null0", prefix)
	}
	if vrf != "" {
		if ipv6 {
			routeCmd = fmt.Sprintf("ipv6 route %s Null0 vrf %s", prefix, vrf)
		} else {
			routeCmd = fmt.Sprintf("ip route %s Null0 vrf %s", prefix, vrf)
		}
	}

	if _, err := c.execVtysh("-c", "configure terminal", "-c", routeCmd); err != nil {
		return fmt.Errorf("install blackhole route: %w", err)
	}

	router := fmt.Sprintf("router bgp %d", asn)
	if vrf != "" {
		router = fmt.Sprintf("router bgp %d vrf %s", asn, vrf)
	}
	_, err := c.execVtysh("-c", "configure terminal",
		"-c", router,
		"-c", fmt.Sprintf("address-family %s unicast", af),
		"-c", fmt.Sprintf("network %s", prefix))
	return err
}

func (c *Component) WithdrawBGPNetwork(asn uint32, vrf string, prefix string, ipv6 bool) error {
	af := "ipv4"
	if ipv6 {
		af = "ipv6"
	}
	router := fmt.Sprintf("router bgp %d", asn)
	if vrf != "" {
		router = fmt.Sprintf("router bgp %d vrf %s", asn, vrf)
	}
	_, err := c.execVtysh("-c", "configure terminal",
		"-c", router,
		"-c", fmt.Sprintf("address-family %s unicast", af),
		"-c", fmt.Sprintf("no network %s", prefix))
	return err
}

func (c *Component) AdvertiseSRGNetworks(ctx context.Context, networks []config.SRGNetwork) error {
	cfg, err := c.configMgr.GetRunning()
	if err != nil {
		return fmt.Errorf("get running config: %w", err)
	}
	if cfg.Protocols.BGP == nil {
		return fmt.Errorf("BGP not configured")
	}
	asn := cfg.Protocols.BGP.ASN
	for _, n := range networks {
		if err := c.AdvertiseBGPNetwork(asn, n.VRF, n.Prefix, strings.Contains(n.Prefix, ":")); err != nil {
			return err
		}
	}
	return nil
}

func (c *Component) WithdrawSRGNetworks(ctx context.Context, networks []config.SRGNetwork) error {
	cfg, err := c.configMgr.GetRunning()
	if err != nil {
		return fmt.Errorf("get running config: %w", err)
	}
	if cfg.Protocols.BGP == nil {
		return fmt.Errorf("BGP not configured")
	}
	asn := cfg.Protocols.BGP.ASN
	for _, n := range networks {
		if err := c.WithdrawBGPNetwork(asn, n.VRF, n.Prefix, strings.Contains(n.Prefix, ":")); err != nil {
			return err
		}
	}
	return nil
}

func (c *Component) execVtysh(args ...string) ([]byte, error) {
	cmd := exec.Command("vtysh", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("routing CP api call failed: %w, output: %s", err, output)
	}

	var warning struct {
		Warning string `json:"warning"`
	}
	if err := json.Unmarshal(output, &warning); err == nil && warning.Warning != "" {
		return nil, fmt.Errorf("routerd: %s", warning.Warning)
	}

	return output, nil
}

func (c *Component) GetVersion(ctx context.Context) (string, error) {
	output, err := c.execVtysh("-c", "show version")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (c *Component) GetVRFs() ([]vrf.VRF, error) {
	vppTables, err := c.southbound.GetIPTables()
	if err != nil {
		return nil, fmt.Errorf("failed to get IP tables: %w", err)
	}

	vppMTables, err := c.southbound.GetIPMTables()
	if err != nil {
		return nil, fmt.Errorf("failed to get IP multicast tables: %w", err)
	}

	tableMap := make(map[uint32]*vrf.VRF)

	for _, t := range vppTables {
		v, exists := tableMap[t.TableID]
		if !exists {
			name := t.Name
			if t.TableID == 0 {
				name = "default"
			}

			v = &vrf.VRF{
				Name:            name,
				TableId:         t.TableID,
				AddressFamilies: vrf.AddressFamilyConfig{},
			}
			tableMap[t.TableID] = v
		}

		if t.IsIPv6 {
			v.AddressFamilies.IPv6Unicast = &vrf.IPv6UnicastAF{}
		} else {
			v.AddressFamilies.IPv4Unicast = &vrf.IPv4UnicastAF{}
		}
	}

	for _, t := range vppMTables {
		v, exists := tableMap[t.TableID]
		if !exists {
			name := t.Name
			if t.TableID == 0 {
				name = "default"
			}

			v = &vrf.VRF{
				Name:            name,
				TableId:         t.TableID,
				AddressFamilies: vrf.AddressFamilyConfig{},
			}
			tableMap[t.TableID] = v
		}

		if t.IsIPv6 {
			v.AddressFamilies.IPv6Multicast = &vrf.IPv6MulticastAF{}
		} else {
			v.AddressFamilies.IPv4Multicast = &vrf.IPv4MulticastAF{}
		}
	}

	var vrfs []vrf.VRF
	for _, v := range tableMap {
		vrfs = append(vrfs, *v)
	}

	return vrfs, nil
}

func (c *Component) CreateVRF() (vrf.VRF, error) {
	tableId, err := c.southbound.GetNextAvailableGlobalTableId()
	if err != nil {
		return vrf.VRF{}, err
	}

	fmt.Println(tableId)

	return vrf.VRF{}, nil
}

func (c *Component) GetBGPStatistics(ipv4 bool) (*bgp.Statistics, error) {
	var (
		cmd          string
		af           string
		wrapperKey   string
	)
	if ipv4 {
		cmd = "show ip bgp statistics json"
		af = "ipv4"
		wrapperKey = "ipv4Unicast"
	} else {
		cmd = "show bgp statistics json"
		af = "ipv6"
		wrapperKey = "ipv6Unicast"
	}

	output, err := c.execVtysh("-c", cmd)
	if err != nil {
		return nil, err
	}

	var wrapper map[string][]bgp.Statistics
	if err := json.Unmarshal(output, &wrapper); err != nil {
		return nil, fmt.Errorf("parse BGP statistics: %w", err)
	}

	result := &bgp.Statistics{AddressFamily: af}
	if entries, ok := wrapper[wrapperKey]; ok && len(entries) > 0 {
		entries[0].AddressFamily = af
		*result = entries[0]
	}
	return result, nil
}

func (c *Component) GetBGPNeighbors() ([]bgp.Neighbor, error) {
	output, err := c.execVtysh("-c", "show bgp neighbors json")
	if err != nil {
		return nil, err
	}

	var raw map[string]bgp.Neighbor
	if err := json.Unmarshal(output, &raw); err != nil {
		return nil, fmt.Errorf("parse BGP neighbors: %w", err)
	}

	out := make([]bgp.Neighbor, 0, len(raw))
	for addr, n := range raw {
		if n.NeighborAddr == "" {
			n.NeighborAddr = addr
		}
		out = append(out, n)
	}
	return out, nil
}

// GetBGPNeighbor returns the per-peer detail for CLI lookups. The
// wildcard show path (protocols.bgp.neighbors.<*:ip>) calls this; the
// aggregate metric path uses GetBGPNeighbors instead.
func (c *Component) GetBGPNeighbor(neighborIP string) (*bgp.Neighbor, error) {
	output, err := c.execVtysh("-c", fmt.Sprintf("show bgp neighbors %s json", neighborIP))
	if err != nil {
		return nil, err
	}

	var raw map[string]bgp.Neighbor
	if err := json.Unmarshal(output, &raw); err != nil {
		return nil, fmt.Errorf("parse BGP neighbor: %w", err)
	}

	if n, ok := raw[neighborIP]; ok {
		if n.NeighborAddr == "" {
			n.NeighborAddr = neighborIP
		}
		return &n, nil
	}
	return nil, nil
}

func (c *Component) GetBGPVPNv4Routes() (*bgp.VPNRoutes, error) {
	return c.fetchVPNRoutes("show bgp ipv4 vpn json", "ipv4")
}

func (c *Component) GetBGPVPNv6Routes() (*bgp.VPNRoutes, error) {
	return c.fetchVPNRoutes("show bgp ipv6 vpn json", "ipv6")
}

func (c *Component) GetBGPVPNv4Summary() (*bgp.VPNSummary, error) {
	return c.fetchVPNSummary("show bgp ipv4 vpn summary json", "ipv4")
}

func (c *Component) GetBGPVPNv6Summary() (*bgp.VPNSummary, error) {
	return c.fetchVPNSummary("show bgp ipv6 vpn summary json", "ipv6")
}

func (c *Component) fetchVPNRoutes(cmd, af string) (*bgp.VPNRoutes, error) {
	output, err := c.execVtysh("-c", cmd)
	if err != nil {
		return nil, err
	}
	var routes bgp.VPNRoutes
	if err := json.Unmarshal(output, &routes); err != nil {
		return nil, fmt.Errorf("parse %q: %w", cmd, err)
	}
	routes.AddressFamily = af
	return &routes, nil
}

func (c *Component) fetchVPNSummary(cmd, af string) (*bgp.VPNSummary, error) {
	output, err := c.execVtysh("-c", cmd)
	if err != nil {
		return nil, err
	}
	var summary bgp.VPNSummary
	if err := json.Unmarshal(output, &summary); err != nil {
		return nil, fmt.Errorf("parse %q: %w", cmd, err)
	}
	summary.AddressFamily = af
	return &summary, nil
}
