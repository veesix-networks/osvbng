package bootstrap

import (
	"fmt"
	"log/slog"

	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

type Bootstrap struct {
	sb     *southbound.VPP
	cfg    *config.Config
	logger *slog.Logger
}

func New(sb *southbound.VPP, cfg *config.Config) *Bootstrap {
	return &Bootstrap{
		sb:     sb,
		cfg:    cfg,
		logger: logger.Component(logger.ComponentBootstrap),
	}
}

func (b *Bootstrap) ProvisionInfrastructure() error {
	b.logger.Info("Provisioning infrastructure from config")

	puntSocketPath := "/run/osvbng/punt.sock"
	if b.cfg.Dataplane.PuntSocketPath != "" {
		puntSocketPath = b.cfg.Dataplane.PuntSocketPath
	}

	if b.cfg.SubscriberGroups != nil {
		for _, group := range b.cfg.SubscriberGroups.Groups {
			for _, vlanRange := range group.VLANs {
				svlans, err := vlanRange.GetSVLANs()
				if err != nil {
					return fmt.Errorf("parse svlan range: %w", err)
				}

				for _, svlan := range svlans {
					b.logger.Info("Creating S-VLAN subinterface", "svlan", svlan, "interface", vlanRange.Interface)

					if err := b.sb.CreateSVLAN(svlan, nil, nil); err != nil {
						return fmt.Errorf("create svlan %d: %w", svlan, err)
					}

					subIfName := fmt.Sprintf("%s.%d", b.sb.GetParentInterface(), svlan)

					if err := b.sb.EnableIPv6(vlanRange.Interface); err != nil {
						b.logger.Debug("Enable IPv6 on loopback", "loopback", vlanRange.Interface, "error", err)
					}

					if err := b.sb.SetUnnumbered(subIfName, vlanRange.Interface); err != nil {
						return fmt.Errorf("set unnumbered %s to %s: %w", subIfName, vlanRange.Interface, err)
					}

					if err := b.sb.EnableARPPunt(subIfName, puntSocketPath); err != nil {
						return fmt.Errorf("enable arp punt on %s: %w", subIfName, err)
					}

					if err := b.sb.EnableDHCPv4Punt(subIfName, puntSocketPath); err != nil {
						return fmt.Errorf("enable dhcp punt on %s: %w", subIfName, err)
					}

					if err := b.sb.EnablePPPoEPunt(subIfName, puntSocketPath); err != nil {
						return fmt.Errorf("enable pppoe punt on %s: %w", subIfName, err)
					}

					if err := b.sb.EnableDHCPv6Punt(subIfName, puntSocketPath); err != nil {
						return fmt.Errorf("enable dhcpv6 punt on %s: %w", subIfName, err)
					}

					if err := b.sb.EnableIPv6(subIfName); err != nil {
						return fmt.Errorf("enable ipv6 on %s: %w", subIfName, err)
					}

					if err := b.sb.EnableDHCPv6Multicast(subIfName); err != nil {
						return fmt.Errorf("enable dhcpv6 multicast on %s: %w", subIfName, err)
					}

					raConfig := b.getRAConfig(group)
					if err := b.sb.ConfigureIPv6RA(subIfName, raConfig); err != nil {
						return fmt.Errorf("configure ipv6 ra on %s: %w", subIfName, err)
					}

					if err := b.sb.DisableARPReply(subIfName); err != nil {
						return fmt.Errorf("disable arp reply on %s: %w", subIfName, err)
					}
				}
			}
		}
	}

	b.logger.Info("Infrastructure provisioning complete")
	return nil
}

func (b *Bootstrap) getRAConfig(group *subscriber.SubscriberGroup) southbound.IPv6RAConfig {
	globalRA := b.cfg.DHCPv6.RA

	managed := globalRA.GetManaged()
	other := globalRA.GetOther()
	routerLifetime := globalRA.GetRouterLifetime()
	maxInterval := globalRA.GetMaxInterval()
	minInterval := globalRA.GetMinInterval()

	if group.IPv6 != nil && group.IPv6.RA != nil {
		groupRA := group.IPv6.RA
		if groupRA.Managed != nil {
			managed = *groupRA.Managed
		}
		if groupRA.Other != nil {
			other = *groupRA.Other
		}
		if groupRA.RouterLifetime != 0 {
			routerLifetime = groupRA.RouterLifetime
		}
		if groupRA.MaxInterval != 0 {
			maxInterval = groupRA.MaxInterval
		}
		if groupRA.MinInterval != 0 {
			minInterval = groupRA.MinInterval
		}
	}

	return southbound.IPv6RAConfig{
		Managed:        managed,
		Other:          other,
		RouterLifetime: routerLifetime,
		MaxInterval:    maxInterval,
		MinInterval:    minInterval,
	}
}

func (b *Bootstrap) Cleanup() error {
	b.logger.Info("Cleaning up provisioned infrastructure")

	if b.cfg.SubscriberGroups != nil {
		for _, group := range b.cfg.SubscriberGroups.Groups {
			for _, vlanRange := range group.VLANs {
				svlans, err := vlanRange.GetSVLANs()
				if err != nil {
					return fmt.Errorf("parse svlan range: %w", err)
				}

				for _, svlan := range svlans {
					vlanName := fmt.Sprintf("vbng.%d", svlan)
					if err := b.sb.DeleteInterface(vlanName); err != nil {
						b.logger.Warn("Failed to delete interface", "interface", vlanName, "error", err)
					}
				}
			}
		}
	}

	b.logger.Info("Cleanup complete")
	return nil
}
