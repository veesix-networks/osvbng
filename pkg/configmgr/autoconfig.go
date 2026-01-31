package configmgr

import (
	"fmt"
	"net"

	"github.com/veesix-networks/osvbng/pkg/autoconfig"
	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/config/protocols"
	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func (cm *ConfigManager) ProcessSubscriberGroups(sessionID conf.SessionID, cfg *config.Config) error {
	if cfg.SubscriberGroups == nil {
		return nil
	}

	for groupName, group := range cfg.SubscriberGroups.Groups {
		if group.BGP != nil && group.BGP.Enabled {
			if err := cm.processBGPForGroup(sessionID, groupName, group); err != nil {
				return fmt.Errorf("subscriber group %s: %w", groupName, err)
			}
		}
	}
	return nil
}

func (cm *ConfigManager) ApplyInfrastructureConfig(sessionID conf.SessionID, cfg *config.Config, parentInterface string) error {
	ac := autoconfig.New(cfg, parentInterface)

	changes, err := ac.DeriveConfig()
	if err != nil {
		return fmt.Errorf("derive infrastructure config: %w", err)
	}

	for _, change := range changes {
		if err := cm.Set(sessionID, change.Path, change.Value); err != nil {
			return fmt.Errorf("apply %s: %w", change.Path, err)
		}
	}

	return nil
}

func (cm *ConfigManager) processBGPForGroup(sessionID conf.SessionID, groupName string, group *subscriber.SubscriberGroup) error {
	vrf := group.BGP.VRF
	if vrf == "" {
		vrf = group.VRF
	}

	if group.BGP.AdvertisePools {
		for _, pool := range group.AddressPools {
			if err := cm.addBGPNetwork(sessionID, pool.Network, vrf); err != nil {
				return fmt.Errorf("pool %s: %w", pool.Name, err)
			}
		}
	}

	if group.BGP.RedistributeConnected {
		if err := cm.enableBGPRedistribute(sessionID, "connected", vrf); err != nil {
			return err
		}
	}

	return nil
}

func (cm *ConfigManager) addBGPNetwork(sessionID conf.SessionID, network, vrf string) error {
	if network == "" {
		return fmt.Errorf("network cannot be empty")
	}

	ip, _, err := net.ParseCIDR(network)
	if err != nil {
		return fmt.Errorf("invalid network %s: %w", network, err)
	}

	var path string
	if ip.To4() != nil {
		if vrf == "" {
			path, err = paths.ProtocolsBGPIPv4UnicastNetwork.Build(network)
		} else {
			path, err = paths.ProtocolsBGPVRFIPv4UnicastNetwork.Build(vrf, network)
		}
	} else {
		if vrf == "" {
			path, err = paths.ProtocolsBGPIPv6UnicastNetwork.Build(network)
		} else {
			path, err = paths.ProtocolsBGPVRFIPv6UnicastNetwork.Build(vrf, network)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to build path: %w", err)
	}

	return cm.Set(sessionID, path, &protocols.BGPNetwork{})
}

func (cm *ConfigManager) enableBGPRedistribute(sessionID conf.SessionID, proto, vrf string) error {
	var path string
	var err error

	if vrf == "" {
		path, err = paths.ProtocolsBGPIPv4UnicastRedistribute.Build()
	} else {
		path, err = paths.ProtocolsBGPVRFIPv4UnicastRedistribute.Build(vrf)
	}

	if err != nil {
		return fmt.Errorf("failed to build path: %w", err)
	}

	redistConfig := &protocols.BGPRedistribute{}

	switch proto {
	case "connected":
		redistConfig.Connected = true
	case "static":
		redistConfig.Static = true
	default:
		return fmt.Errorf("unknown redistribute protocol: %s", proto)
	}

	return cm.Set(sessionID, path, redistConfig)
}
