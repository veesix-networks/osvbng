package routing

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models/protocols/bgp"
	"github.com/veesix-networks/osvbng/pkg/models/vrf"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

type Component struct {
	*component.Base
	logger     *slog.Logger
	southbound *southbound.VPP
}

func New(deps component.Dependencies) (component.Component, error) {
	log := logger.Get(logger.Routing)

	c := &Component{
		Base:       component.NewBase("routing"),
		logger:     log,
		southbound: deps.VPP,
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
	var output []byte
	var err error

	if ipv4 {
		output, err = c.execVtysh("-c", "show ip bgp statistics json")
	} else {
		output, err = c.execVtysh("-c", "show bgp statistics json")
	}
	if err != nil {
		return nil, err
	}

	var stats map[string][]bgpStatisticsEntry
	if err := json.Unmarshal(output, &stats); err != nil {
		return nil, fmt.Errorf("parse BGP statistics: %w", err)
	}

	result := &bgp.Statistics{}

	if ipv4 {
		if ipv4Stats, ok := stats["ipv4Unicast"]; ok && len(ipv4Stats) > 0 {
			result.Instance = ipv4Stats[0].Instance
			result.TotalPrefixes = ipv4Stats[0].TotalPrefixes
			result.TotalAdvertisements = ipv4Stats[0].TotalAdvertisements
			result.AveragePrefixLength = ipv4Stats[0].AveragePrefixLength
		}
	} else {
		if ipv6Stats, ok := stats["ipv6Unicast"]; ok && len(ipv6Stats) > 0 {
			result.Instance = ipv6Stats[0].Instance
			result.TotalPrefixes = ipv6Stats[0].TotalPrefixes
			result.TotalAdvertisements = ipv6Stats[0].TotalAdvertisements
			result.AveragePrefixLength = ipv6Stats[0].AveragePrefixLength
		}
	}

	return result, nil
}

func (c *Component) GetBGPNeighbor(neighborIP string) (interface{}, error) {
	output, err := c.execVtysh("-c", fmt.Sprintf("show bgp neighbors %s json", neighborIP))
	if err != nil {
		return nil, err
	}

	var resultMap map[string]interface{}
	if err := json.Unmarshal(output, &resultMap); err != nil {
		return nil, fmt.Errorf("parse BGP neighbor: %w", err)
	}

	if neighbor, ok := resultMap[neighborIP]; ok {
		return neighbor, nil
	}

	return resultMap, nil
}

func (c *Component) GetBGPVPNv4Routes() (json.RawMessage, error) {
	output, err := c.execVtysh("-c", "show bgp ipv4 vpn json")
	if err != nil {
		return nil, err
	}
	return json.RawMessage(output), nil
}

func (c *Component) GetBGPVPNv6Routes() (json.RawMessage, error) {
	output, err := c.execVtysh("-c", "show bgp ipv6 vpn json")
	if err != nil {
		return nil, err
	}
	return json.RawMessage(output), nil
}

func (c *Component) GetBGPVPNv4Summary() (json.RawMessage, error) {
	output, err := c.execVtysh("-c", "show bgp ipv4 vpn summary json")
	if err != nil {
		return nil, err
	}
	return json.RawMessage(output), nil
}

func (c *Component) GetBGPVPNv6Summary() (json.RawMessage, error) {
	output, err := c.execVtysh("-c", "show bgp ipv6 vpn summary json")
	if err != nil {
		return nil, err
	}
	return json.RawMessage(output), nil
}

type bgpStatisticsEntry struct {
	Instance                string  `json:"instance"`
	TotalPrefixes           int     `json:"totalPrefixes"`
	TotalAdvertisements     int     `json:"totalAdvertisements"`
	AveragePrefixLength     float64 `json:"averagePrefixLength"`
	LongestAsPath           int     `json:"longestAsPath"`
	AverageAsPathLengthHops float64 `json:"averageAsPathLengthHops"`
}
