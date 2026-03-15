// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package cgnat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"

	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/config/cgnat"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/ifmgr"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/opdb"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/vrfmgr"
)

const opdbNamespace = "cgnat_mappings"

type Component struct {
	*component.Base

	logger    *slog.Logger
	eventBus  events.Bus
	dataplane southbound.Southbound
	opdb      opdb.Store
	cfgMgr    component.ConfigManager
	ifMgr     *ifmgr.Manager
	vrfMgr    *vrfmgr.Manager

	pools     *PoolManager
	reverse   *ReverseIndex
	bypass    *BypassManager
	blacklist *BlacklistManager

	poolIDMap  map[string]uint32
	nextPoolID uint32

	lifecycleSub events.Subscription
}

func NewComponent(deps component.Dependencies, ifMgr *ifmgr.Manager, vrfMgr *vrfmgr.Manager) (*Component, error) {
	c := &Component{
		Base:      component.NewBase("cgnat"),
		logger:    logger.Get(logger.CGNAT),
		eventBus:  deps.EventBus,
		dataplane: deps.Southbound,
		opdb:      deps.OpDB,
		cfgMgr:    deps.ConfigManager,
		ifMgr:     ifMgr,
		vrfMgr:    vrfMgr,
		pools:     NewPoolManager(),
		reverse:   NewReverseIndex(),
		bypass:    NewBypassManager(),
		blacklist: NewBlacklistManager(),
		poolIDMap: make(map[string]uint32),
	}

	return c, nil
}

func (c *Component) Start(ctx context.Context) error {
	c.StartContext(ctx)
	c.logger.Info("Starting CGNAT component")

	cfg, err := c.cfgMgr.GetRunning()
	if err != nil {
		return fmt.Errorf("get running config: %w", err)
	}

	if cfg.CGNAT == nil {
		c.logger.Info("No CGNAT configuration, component idle")
		return nil
	}

	if err := c.configurePools(cfg.CGNAT); err != nil {
		return fmt.Errorf("configure pools: %w", err)
	}

	if err := c.setupOutsideInterfaces(cfg); err != nil {
		c.logger.Warn("Failed to setup outside interfaces", "error", err)
	}

	if err := c.restoreFromOpDB(); err != nil {
		c.logger.Warn("Failed to restore CGNAT state from OpDB", "error", err)
	}

	c.lifecycleSub = c.eventBus.Subscribe(events.TopicSessionLifecycle, c.handleSessionLifecycle)

	c.logger.Info("CGNAT component started", "pools", len(cfg.CGNAT.Pools))
	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	c.logger.Info("Stopping CGNAT component")
	if c.lifecycleSub != nil {
		c.lifecycleSub.Unsubscribe()
	}
	c.StopContext()
	return nil
}

func (c *Component) configurePools(cfg *cgnat.Config) error {
	for name, poolCfg := range cfg.Pools {
		c.nextPoolID++
		poolID := c.nextPoolID
		c.poolIDMap[name] = poolID

		if err := c.pools.ConfigurePool(name, poolID, poolCfg); err != nil {
			return fmt.Errorf("pool %s: %w", name, err)
		}

		var mode uint8
		if poolCfg.GetMode() == "deterministic" {
			mode = 1
		}
		var addrPooling uint8
		if poolCfg.GetAddressPooling() == "arbitrary" {
			addrPooling = 1
		}
		var filtering uint8
		if poolCfg.GetFiltering() == "endpoint-dependent" {
			filtering = 1
		}

		timeouts := poolCfg.GetTimeouts()
		t := [4]uint32{timeouts.TCPEstablished, timeouts.TCPTransitory, timeouts.UDP, timeouts.ICMP}

		if err := c.dataplane.CGNATPoolAddDel(poolID, mode, addrPooling, filtering,
			poolCfg.GetBlockSize(), poolCfg.GetMaxBlocksPerSubscriber(),
			poolCfg.GetMaxSessionsPerSubscriber(), poolCfg.GetPortRangeStart(),
			poolCfg.GetPortRangeEnd(), poolCfg.GetPortReuseTimeout(),
			poolCfg.GetALGBitmask(), t, true); err != nil {
			return fmt.Errorf("pool add %s: %w", name, err)
		}

		for _, prefix := range poolCfg.InsidePrefixes {
			_, ipNet, err := net.ParseCIDR(prefix.Prefix)
			if err != nil {
				return fmt.Errorf("pool %s inside prefix %s: %w", name, prefix.Prefix, err)
			}
			vrfID := uint32(0)
			if prefix.VRF != "" && c.vrfMgr != nil {
				if tableID, _, _, err := c.vrfMgr.ResolveVRF(prefix.VRF); err == nil {
					vrfID = tableID
				}
			}
			if err := c.dataplane.CGNATPoolAddInsidePrefix(poolID, *ipNet, vrfID, true); err != nil {
				return fmt.Errorf("inside prefix: %w", err)
			}
		}

		for _, addrStr := range poolCfg.OutsideAddresses {
			_, ipNet, err := net.ParseCIDR(addrStr)
			if err != nil {
				ip := net.ParseIP(addrStr)
				if ip == nil {
					return fmt.Errorf("pool %s outside address %s: invalid", name, addrStr)
				}
				ipNet = &net.IPNet{IP: ip.To4(), Mask: net.CIDRMask(32, 32)}
			}
			if err := c.dataplane.CGNATPoolAddOutsideAddress(poolID, *ipNet, true); err != nil {
				return fmt.Errorf("outside address: %w", err)
			}
		}

		for _, excluded := range poolCfg.ExcludedAddresses {
			ip := net.ParseIP(excluded)
			if ip != nil {
				c.blacklist.Exclude(name, ip)
			}
		}

		c.logger.Info("Pool configured", "name", name, "id", poolID, "mode", poolCfg.GetMode())
	}

	return nil
}

func (c *Component) setupOutsideInterfaces(cfg *config.Config) error {
	coreIfName := cfg.GetCoreInterface()
	if coreIfName == "" {
		c.logger.Warn("No core interface configured, outside VRF not set")
		return nil
	}

	swIfIndex, ok := c.ifMgr.GetSwIfIndex(coreIfName)
	if !ok {
		return fmt.Errorf("core interface %s not found in dataplane", coreIfName)
	}

	iface := c.ifMgr.Get(swIfIndex)

	var vrfTableID uint32
	if iface != nil {
		vrfTableID = iface.FIBTableID
	}

	for poolName, poolID := range c.poolIDMap {
		if err := c.dataplane.CGNATSetOutsideVRF(poolID, vrfTableID); err != nil {
			return fmt.Errorf("set outside VRF for pool %s: %w", poolName, err)
		}

		c.logger.Info("Outside VRF configured",
			"pool", poolName,
			"interface", coreIfName,
			"vrf_table_id", vrfTableID)
	}

	return nil
}

func (c *Component) handleSessionLifecycle(event events.Event) {
	data, ok := event.Data.(*events.SessionLifecycleEvent)
	if !ok {
		return
	}

	switch data.State {
	case models.SessionStateActive:
		c.handleSessionActivate(data)
	case models.SessionStateReleased:
		c.handleSessionRelease(data)
	}
}

func (c *Component) handleSessionActivate(data *events.SessionLifecycleEvent) {
	var insideIP net.IP
	var vrfName string
	var swIfIndex uint32
	var serviceGroup string

	switch data.AccessType {
	case models.AccessTypeIPoE:
		sess, ok := data.Session.(*models.IPoESession)
		if !ok {
			return
		}
		insideIP = sess.IPv4Address
		vrfName = sess.VRF
		swIfIndex = sess.IfIndex
		serviceGroup = sess.ServiceGroup
	case models.AccessTypePPPoE:
		sess, ok := data.Session.(*models.PPPSession)
		if !ok {
			return
		}
		insideIP = sess.IPv4Address
		vrfName = sess.VRF
		swIfIndex = sess.IfIndex
		serviceGroup = sess.ServiceGroup
	default:
		return
	}

	if insideIP == nil || insideIP.To4() == nil {
		return
	}

	cfg, err := c.cfgMgr.GetRunning()
	if err != nil || cfg.CGNAT == nil {
		return
	}

	if serviceGroup != "" && cfg.ServiceGroups != nil {
		if sg, ok := cfg.ServiceGroups[serviceGroup]; ok && sg.CGNAT != nil {
			if sg.CGNAT.Bypass {
				c.handleBypass(insideIP, vrfName)
				return
			}
			if sg.CGNAT.Policy != "" {
				c.handlePBAActivate(sg.CGNAT.Policy, insideIP, vrfName, swIfIndex, data.SessionID)
				return
			}
		}
	}

	poolName := c.pools.FindPoolForIP(insideIP, vrfName)
	if poolName == "" {
		return
	}

	pool := cfg.CGNAT.Pools[poolName]
	if pool == nil {
		return
	}

	if pool.GetMode() == "deterministic" {
		c.handleDetActivate(poolName, swIfIndex)
	} else {
		c.handlePBAActivate(poolName, insideIP, vrfName, swIfIndex, data.SessionID)
	}
}

func (c *Component) handlePBAActivate(poolName string, insideIP net.IP, vrfName string, swIfIndex uint32, sessionID string) {
	mapping, err := c.pools.AllocateBlock(poolName, insideIP, 0, swIfIndex)
	if err != nil {
		c.logger.Error("CGNAT block allocation failed", "pool", poolName, "ip", insideIP, "error", err)
		return
	}

	poolID := c.poolIDMap[poolName]

	c.dataplane.CGNATAddDelSubscriberMappingAsync(poolID, swIfIndex, insideIP,
		0, mapping.OutsideIP, mapping.PortBlockStart, mapping.PortBlockEnd,
		true, true, func(err error) {
			if err != nil {
				c.logger.Error("subscriber mapping failed, rolling back", "error", err)
				c.pools.ReleaseBlocks(poolName, insideIP, 0)
				return
			}

			c.reverse.Add(mapping)

			if c.opdb != nil {
				if data, err := json.Marshal(mapping); err == nil {
					c.opdb.Put(context.Background(), opdbNamespace, sessionID, data)
				}
			}

			c.logger.Debug("CGNAT PBA mapping created",
				"session", sessionID,
				"inside", insideIP,
				"outside", fmt.Sprintf("%s:%d-%d", mapping.OutsideIP, mapping.PortBlockStart, mapping.PortBlockEnd),
				"pool", poolName)
		})
}

func (c *Component) handleDetActivate(poolName string, swIfIndex uint32) {
	poolID, ok := c.poolIDMap[poolName]
	if !ok {
		return
	}

	if err := c.dataplane.CGNATEnableOnSession(poolID, swIfIndex, true); err != nil {
		c.logger.Error("enable CGNAT on session failed", "pool", poolName, "sw_if", swIfIndex, "error", err)
	}
}

func (c *Component) handleBypass(insideIP net.IP, vrfName string) {
	prefix := net.IPNet{IP: insideIP.To4(), Mask: net.CIDRMask(32, 32)}
	if err := c.dataplane.CGNATAddDelBypass(prefix, 0, true); err != nil {
		c.logger.Error("bypass programming failed", "ip", insideIP, "error", err)
		return
	}
	c.bypass.AddIP(insideIP, 0)
}

func (c *Component) handleSessionRelease(data *events.SessionLifecycleEvent) {
	var insideIP net.IP
	var swIfIndex uint32
	var serviceGroup string

	switch data.AccessType {
	case models.AccessTypeIPoE:
		sess, ok := data.Session.(*models.IPoESession)
		if !ok {
			return
		}
		insideIP = sess.IPv4Address
		swIfIndex = sess.IfIndex
		serviceGroup = sess.ServiceGroup
	case models.AccessTypePPPoE:
		sess, ok := data.Session.(*models.PPPSession)
		if !ok {
			return
		}
		insideIP = sess.IPv4Address
		swIfIndex = sess.IfIndex
		serviceGroup = sess.ServiceGroup
	default:
		return
	}

	if insideIP == nil || insideIP.To4() == nil {
		return
	}

	cfg, _ := c.cfgMgr.GetRunning()
	if cfg != nil && cfg.CGNAT != nil && serviceGroup != "" && cfg.ServiceGroups != nil {
		if sg, ok := cfg.ServiceGroups[serviceGroup]; ok && sg.CGNAT != nil && sg.CGNAT.Bypass {
			prefix := net.IPNet{IP: insideIP.To4(), Mask: net.CIDRMask(32, 32)}
			c.dataplane.CGNATAddDelBypass(prefix, 0, false)
			c.bypass.RemovePrefix(&prefix, 0)
			return
		}
	}

	mappings := c.pools.GetMappings("", insideIP, 0)
	if len(mappings) == 0 {
		if err := c.dataplane.CGNATEnableOnSession(0, swIfIndex, false); err != nil {
			c.logger.Debug("Disable CGNAT on session", "sw_if", swIfIndex, "error", err)
		}
		return
	}

	poolName := mappings[0].PoolName
	poolID := c.poolIDMap[poolName]

	for _, m := range mappings {
		c.dataplane.CGNATAddDelSubscriberMappingAsync(poolID, swIfIndex, insideIP,
			0, m.OutsideIP, m.PortBlockStart, m.PortBlockEnd,
			false, false, func(err error) {
				if err != nil {
					c.logger.Error("remove mapping failed", "error", err)
				}
			})
		c.reverse.Remove(m.OutsideIP, m.PortBlockStart)
	}

	c.pools.ReleaseBlocks(poolName, insideIP, 0)

	if c.opdb != nil {
		c.opdb.Delete(context.Background(), opdbNamespace, data.SessionID)
	}

	c.logger.Debug("CGNAT mappings released", "session", data.SessionID, "inside", insideIP, "blocks", len(mappings))
}

func (c *Component) restoreFromOpDB() error {
	if c.opdb == nil {
		return nil
	}

	restored := 0
	err := c.opdb.Load(context.Background(), opdbNamespace, func(key string, value []byte) error {
		var mapping models.CGNATMapping
		if err := json.Unmarshal(value, &mapping); err != nil {
			c.logger.Warn("Failed to unmarshal mapping", "key", key, "error", err)
			return nil
		}

		if err := c.pools.RestoreMapping(&mapping); err != nil {
			c.logger.Warn("Failed to restore mapping", "inside", mapping.InsideIP, "error", err)
			return nil
		}

		c.reverse.Add(&mapping)
		restored++
		return nil
	})

	if err != nil {
		return err
	}

	if restored > 0 {
		c.logger.Info("Restored CGNAT mappings from OpDB", "count", restored)
	}
	return nil
}

func (c *Component) GetRunningConfig() (*cgnat.Config, error) {
	cfg, err := c.cfgMgr.GetRunning()
	if err != nil {
		return nil, err
	}
	return cfg.CGNAT, nil
}

func (c *Component) GetPoolManager() *PoolManager {
	return c.pools
}

func (c *Component) GetReverseIndex() *ReverseIndex {
	return c.reverse
}

func (c *Component) GetBypassManager() *BypassManager {
	return c.bypass
}

func (c *Component) GetBlacklistManager() *BlacklistManager {
	return c.blacklist
}
