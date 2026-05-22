package routing

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
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

func (c *Component) GetBGPVPNRoutes(vrf, afi string) (*bgp.VPNRoutes, error) {
	if afi != "ipv4" && afi != "ipv6" {
		return nil, fmt.Errorf("invalid BGP AFI %q", afi)
	}
	prefix, err := bgpVRFPrefix(vrf)
	if err != nil {
		return nil, err
	}
	return c.fetchVPNRoutes("show bgp "+prefix+afi+" vpn json", afi)
}

func (c *Component) GetBGPVPNSummary(vrf, afi string) (*bgp.VPNSummary, error) {
	if afi != "ipv4" && afi != "ipv6" {
		return nil, fmt.Errorf("invalid BGP AFI %q", afi)
	}
	prefix, err := bgpVRFPrefix(vrf)
	if err != nil {
		return nil, err
	}
	return c.fetchVPNSummary("show bgp "+prefix+afi+" vpn summary json", afi)
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

var validBGPVRFRE = regexp.MustCompile(`^(all|[A-Za-z0-9_-]+)$`)

func bgpVRFPrefix(vrf string) (string, error) {
	if vrf == "" {
		return "", nil
	}
	if !validBGPVRFRE.MatchString(vrf) {
		return "", fmt.Errorf("invalid VRF name %q", vrf)
	}
	return "vrf " + vrf + " ", nil
}

func (c *Component) GetBGPSummary(vrf string) (json.RawMessage, error) {
	prefix, err := bgpVRFPrefix(vrf)
	if err != nil {
		return nil, err
	}
	output, err := c.execVtysh("-c", "show bgp "+prefix+"summary json")
	if err != nil {
		return nil, err
	}
	return json.RawMessage(output), nil
}

func (c *Component) GetBGPSummaryAll() (json.RawMessage, error) {
	output, err := c.execVtysh("-c", "show bgp vrf all summary json")
	if err != nil {
		return nil, err
	}
	return json.RawMessage(output), nil
}

func (c *Component) GetBGPAFISummary(vrf, afi string) (*bgp.SummaryAFI, error) {
	if afi != "ipv4" && afi != "ipv6" {
		return nil, fmt.Errorf("invalid BGP AFI %q", afi)
	}
	prefix, err := bgpVRFPrefix(vrf)
	if err != nil {
		return nil, err
	}
	output, err := c.execVtysh("-c", "show bgp "+prefix+afi+" unicast summary json")
	if err != nil {
		return nil, err
	}
	var s bgp.SummaryAFI
	if err := json.Unmarshal(output, &s); err != nil {
		return nil, fmt.Errorf("parse BGP %s unicast summary: %w", afi, err)
	}
	return &s, nil
}

func (c *Component) GetBGPAFISummaryAll(afi string) (map[string]bgp.SummaryAFI, error) {
	if afi != "ipv4" && afi != "ipv6" {
		return nil, fmt.Errorf("invalid BGP AFI %q", afi)
	}
	output, err := c.execVtysh("-c", "show bgp vrf all "+afi+" unicast summary json")
	if err != nil {
		return nil, err
	}
	out := map[string]bgp.SummaryAFI{}
	if err := json.Unmarshal(output, &out); err != nil {
		return nil, fmt.Errorf("parse BGP %s unicast summary (vrf all): %w", afi, err)
	}
	return out, nil
}

func (c *Component) GetBGPStatisticsAll(ipv4 bool) (json.RawMessage, error) {
	afi := "ipv4"
	if !ipv4 {
		afi = "ipv6"
	}
	output, err := c.execVtysh("-c", "show bgp vrf all "+afi+" unicast statistics json")
	if err != nil {
		return nil, err
	}
	return json.RawMessage(output), nil
}

var validBGPRIBFilters = map[string]struct{}{
	"":               {},
	"self-originate": {},
	"cidr-only":      {},
	"detail":         {},
}

var validBGPNeighborViews = map[string]struct{}{
	"advertised-routes": {},
	"received-routes":   {},
	"routes":            {},
}

func (c *Component) GetBGPNeighborsAll() (json.RawMessage, error) {
	output, err := c.execVtysh("-c", "show bgp vrf all neighbors json")
	if err != nil {
		return nil, err
	}
	return json.RawMessage(output), nil
}

func (c *Component) GetBGPImportCheckTable(vrf string, detail bool) (json.RawMessage, error) {
	prefix, err := bgpVRFPrefix(vrf)
	if err != nil {
		return nil, err
	}
	cmd := "show bgp " + prefix + "import-check-table"
	if detail {
		cmd += " detail"
	}
	output, err := c.execVtysh("-c", cmd+" json")
	if err != nil {
		return nil, err
	}
	return json.RawMessage(output), nil
}

func (c *Component) GetBGPNeighborRoutes(vrf, neighbor, view string) (json.RawMessage, error) {
	if _, ok := validBGPNeighborViews[view]; !ok {
		return nil, fmt.Errorf("invalid BGP neighbor view %q", view)
	}
	prefix, err := bgpVRFPrefix(vrf)
	if err != nil {
		return nil, err
	}
	output, err := c.execVtysh("-c", "show bgp "+prefix+"neighbors "+neighbor+" "+view+" json")
	if err != nil {
		return nil, err
	}
	return json.RawMessage(output), nil
}

func (c *Component) GetBGPNexthop(vrf, afi string, detail bool) (json.RawMessage, error) {
	if afi != "" && afi != "ipv4" && afi != "ipv6" {
		return nil, fmt.Errorf("invalid BGP nexthop AFI %q", afi)
	}
	prefix, err := bgpVRFPrefix(vrf)
	if err != nil {
		return nil, err
	}
	cmd := "show bgp " + prefix + "nexthop"
	if afi != "" {
		cmd += " " + afi
	}
	if detail {
		cmd += " detail"
	}
	output, err := c.execVtysh("-c", cmd+" json")
	if err != nil {
		return nil, err
	}
	return json.RawMessage(output), nil
}

func (c *Component) GetBGPRIB(vrf, afi, filter string) (json.RawMessage, error) {
	if afi != "ipv4" && afi != "ipv6" {
		return nil, fmt.Errorf("invalid BGP AFI %q", afi)
	}
	if _, ok := validBGPRIBFilters[filter]; !ok {
		return nil, fmt.Errorf("invalid BGP RIB filter %q", filter)
	}
	prefix, err := bgpVRFPrefix(vrf)
	if err != nil {
		return nil, err
	}
	cmd := "show bgp " + prefix + afi + " unicast"
	if filter != "" {
		cmd += " " + filter
	}
	output, err := c.execVtysh("-c", cmd+" json")
	if err != nil {
		return nil, err
	}
	return json.RawMessage(output), nil
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
