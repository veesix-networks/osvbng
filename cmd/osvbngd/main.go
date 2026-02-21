package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/veesix-networks/osvbng/internal/aaa"
	"github.com/veesix-networks/osvbng/internal/arp"
	"github.com/veesix-networks/osvbng/internal/dataplane"
	"github.com/veesix-networks/osvbng/internal/gateway"
	"github.com/veesix-networks/osvbng/internal/ipoe"
	"github.com/veesix-networks/osvbng/internal/monitor"
	"github.com/veesix-networks/osvbng/internal/pppoe"
	"github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/internal/subscriber"
	"github.com/veesix-networks/osvbng/internal/watchdog"
	"github.com/veesix-networks/osvbng/internal/watchdog/targets"
	syscfg "github.com/veesix-networks/osvbng/pkg/config/system"
	"github.com/veesix-networks/osvbng/pkg/auth"
	"github.com/veesix-networks/osvbng/pkg/cache/memory"
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/configmgr"
	"github.com/veesix-networks/osvbng/pkg/cppm"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/dhcp4"
	"github.com/veesix-networks/osvbng/pkg/dhcp6"
	"github.com/veesix-networks/osvbng/pkg/events/local"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/conf/all"
	"github.com/veesix-networks/osvbng/pkg/handlers/oper"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/oper/all"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/show/all"
	"github.com/veesix-networks/osvbng/pkg/ifmgr"
	"github.com/veesix-networks/osvbng/pkg/svcgroup"
	"github.com/veesix-networks/osvbng/pkg/vrfmgr"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/northbound"
	"github.com/veesix-networks/osvbng/pkg/opdb/sqlite"
	"github.com/veesix-networks/osvbng/pkg/southbound/vpp"
	"github.com/veesix-networks/osvbng/pkg/state"
	"github.com/veesix-networks/osvbng/pkg/version"
	_ "github.com/veesix-networks/osvbng/plugins/all"
	"go.fd.io/govpp"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "Path to configuration file")
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.Full())
		return
	}

	if len(flag.Args()) > 0 && flag.Args()[0] == "config" {
		allDataplane := false
		args := flag.Args()[1:]
		for i := 0; i < len(args); i++ {
			if args[i] == "--all" && i+1 < len(args) && args[i+1] == "dataplane" {
				allDataplane = true
				break
			}
		}

		output, err := config.Generate(config.GenerateOptions{AllDataplane: allDataplane})
		if err != nil {
			log.Fatalf("Failed to generate config: %v", err)
		}
		os.Stdout.WriteString(output)
		return
	}

	if len(flag.Args()) > 0 && flag.Args()[0] == "generate-external" {
		if err := config.GenerateExternalConfigs(*configPath); err != nil {
			log.Fatalf("Failed to generate external configs: %v", err)
		}
		return
	}

	if _, err := os.Stat(*configPath); os.IsNotExist(err) {
		log.Printf("Config file not found at %s, generating default config", *configPath)
		defaultCfg, err := config.Generate(config.GenerateOptions{AllDataplane: true})
		if err != nil {
			log.Fatalf("Failed to generate default config: %v", err)
		}
		if err := os.WriteFile(*configPath, []byte(defaultCfg), 0644); err != nil {
			log.Fatalf("Failed to write default config: %v", err)
		}
		log.Printf("Default config written to %s", *configPath)
	}

	configd := configmgr.NewConfigManager()

	cfg, err := configd.LoadStartupConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	logger.Configure(cfg.Logging.Format, cfg.Logging.Level, cfg.Logging.Components)

	mainLog := logger.Get(logger.Main)

	mainLog.Info("Starting osvbng")

	apiSocket := cfg.Dataplane.DPAPISocket
	if apiSocket == "" {
		apiSocket = "/run/osvbng/dataplane_api.sock"
	}

	vppConn, err := govpp.Connect(apiSocket)
	if err != nil {
		log.Fatalf("Failed to connect to VPP: %v", err)
	}

	accessInterface, err := cfg.GetAccessInterface()
	if err != nil {
		log.Fatalf("Invalid access interface configuration: %v", err)
	}

	ifMgr := ifmgr.New()

	vpp, err := vpp.NewVPP(vpp.VPPConfig{
		Connection: vppConn,
		IfMgr:      ifMgr,
		UseDPDK:    cfg.Dataplane.DPDK != nil && len(cfg.Dataplane.DPDK.Devices) > 0,
	})
	if err != nil {
		log.Fatalf("Failed to create VPP southbound: %v", err)
	}

	vrfMgr := vrfmgr.New(vpp)
	vpp.SetVRFResolver(vrfMgr.ResolveVRF)

	if err := vpp.SetLCPNetNs(config.LCPNetNs); err != nil {
		mainLog.Warn("LCP netns not available, LCP interfaces will use default namespace", "ns", config.LCPNetNs, "error", err)
	}

	nsHandle, err := netns.GetFromName(config.LCPNetNs)
	if err != nil {
		mainLog.Warn("Failed to get LCP netns for VRF manager", "ns", config.LCPNetNs, "error", err)
	} else {
		nlHandle, err := netlink.NewHandleAt(nsHandle)
		if err != nil {
			mainLog.Warn("Failed to create netlink handle for VRF manager", "ns", config.LCPNetNs, "error", err)
		} else {
			vrfMgr.SetNetlinkHandle(nlHandle)
			mainLog.Info("VRF manager configured for LCP namespace", "ns", config.LCPNetNs)
		}
	}

	svcGroupResolver := svcgroup.New()

	cppmManager := cppm.NewManager(cppm.DefaultConfig())

	if err := configd.LoadVersions(); err != nil {
		mainLog.Warn("Failed to load config versions", "error", err)
	}

	configd.AutoRegisterHandlers(&deps.ConfDeps{
		DataplaneState:   nil,
		Southbound:       vpp,
		AAA:              nil,
		CPPM:             cppmManager,
		Routing:          nil,
		VRFManager:       vrfMgr,
		SvcGroupResolver: svcGroupResolver,
		PluginComponents: nil,
	})

	if err := bootstrapDataplane(mainLog, configd, vpp, vrfMgr, svcGroupResolver, cppmManager, cfg, accessInterface); err != nil {
		log.Fatalf("Failed to bootstrap dataplane: %v", err)
	}

	eventBus := local.NewBus()
	cache := memory.New()

	opdbStore, err := sqlite.Open("/var/lib/osvbng/opdb.db")
	if err != nil {
		log.Fatalf("Failed to open OpDB: %v", err)
	}
	defer opdbStore.Close()
	mainLog.Info("OpDB initialized", "path", "/var/lib/osvbng/opdb.db")

	coreDeps := component.Dependencies{
		EventBus:         eventBus,
		Cache:            cache,
		Southbound:       vpp,
		VRFManager:       vrfMgr,
		SvcGroupResolver: svcGroupResolver,
		ConfigManager:    configd,
		OpDB:             opdbStore,
		CPPM:             cppmManager,
	}

	dataplaneComp, err := dataplane.New(coreDeps)
	if err != nil {
		log.Fatalf("Failed to create dataplane component: %v", err)
	}

	dpComp := dataplaneComp.(*dataplane.Component)
	coreDeps.DHCPChan = dpComp.DHCPChan
	coreDeps.DHCPv6Chan = dpComp.DHCPv6Chan
	coreDeps.ARPChan = dpComp.ARPChan
	coreDeps.PPPChan = dpComp.PPPoEChan
	coreDeps.IPv6NDChan = dpComp.IPv6NDChan

	authProviderName := cfg.AAA.AuthProvider
	if authProviderName == "" {
		authProviderName = "local"
	}

	authProvider, err := auth.New(authProviderName, cfg)
	if err != nil {
		log.Fatalf("Failed to create auth provider '%s': %v", authProviderName, err)
	}

	aaaComp, err := aaa.New(coreDeps, authProvider)
	if err != nil {
		log.Fatalf("Failed to create AAA component: %v", err)
	}

	routingComp, err := routing.New(coreDeps)
	if err != nil {
		log.Fatalf("Failed to create routing component: %v", err)
	}

	dhcp4ProviderName := cfg.DHCP.Provider
	if dhcp4ProviderName == "" {
		dhcp4ProviderName = "local"
	}

	dhcp4Factory, ok := dhcp4.Get(dhcp4ProviderName)
	if !ok {
		log.Fatalf("DHCP4 provider '%s' not found. Available providers: %v", dhcp4ProviderName, dhcp4.List())
	}

	dhcp4Provider, err := dhcp4Factory(cfg)
	if err != nil {
		log.Fatalf("Failed to create DHCP4 provider '%s': %v", dhcp4ProviderName, err)
	}

	dhcp6ProviderName := cfg.DHCPv6.Provider
	if dhcp6ProviderName == "" {
		dhcp6ProviderName = "local"
	}

	var dhcp6Provider dhcp6.DHCPProvider
	if dhcp6Factory, ok := dhcp6.Get(dhcp6ProviderName); ok {
		dhcp6Provider, err = dhcp6Factory(cfg)
		if err != nil {
			log.Fatalf("Failed to create DHCP6 provider '%s': %v", dhcp6ProviderName, err)
		}
	}

	ipoeComp, err := ipoe.New(coreDeps, nil, ifMgr, dhcp4Provider, dhcp6Provider)
	if err != nil {
		log.Fatalf("Failed to create ipoe component: %v", err)
	}

	subscriberComp, err := subscriber.New(coreDeps, nil)
	if err != nil {
		log.Fatalf("Failed to create subscriber component: %v", err)
	}

	arpComp, err := arp.New(coreDeps, nil, ifMgr)
	if err != nil {
		log.Fatalf("Failed to create arp component: %v", err)
	}

	pppoeComp, err := pppoe.New(coreDeps, nil, ifMgr)
	if err != nil {
		log.Fatalf("Failed to create pppoe component: %v", err)
	}

	wdCfg := cfg.Watchdog

	var wd *watchdog.Watchdog
	if wdCfg.Enabled {
		wd = watchdog.New()

		if tc := wdCfg.Targets["vpp"]; tc == nil || tc.Enabled {
			vppTarget := targets.NewVPPTarget(vpp, apiSocket, isCritical(wdCfg.Targets["vpp"]))
			vppTarget.SetCallbacks(targets.VPPCallbacks{
				OnDown: func() {
					mainLog.Error("VPP is DOWN, pausing dataplane")
					dpComp.PauseProcessing()
				},
				OnUp: func() {
					mainLog.Info("VPP is UP, resuming dataplane")
					dpComp.ResumeProcessing()
				},
				OnRecover: func(ctx context.Context) error {
					mainLog.Info("VPP recovery: reconnecting southbound")
					if err := vpp.Reconnect(apiSocket); err != nil {
						return fmt.Errorf("VPP reconnect: %w", err)
					}

					ifCount := len(vpp.GetIfMgr().List())
					mainLog.Info("VPP recovery: checked dataplane state", "interfaces", ifCount)

					if ifCount <= 1 {
						mainLog.Info("VPP recovery: dataplane state lost, bootstrapping")
						if err := bootstrapDataplane(mainLog, configd, vpp, vrfMgr, svcGroupResolver, cppmManager, cfg, accessInterface); err != nil {
							return fmt.Errorf("bootstrap dataplane: %w", err)
						}

						mainLog.Info("VPP recovery: reconnecting dataplane SHM")
						if err := dpComp.Reconnect(); err != nil {
							return fmt.Errorf("dataplane reconnect: %w", err)
						}

						mainLog.Info("VPP recovery: recovering IPoE sessions")
						if err := ipoeComp.(*ipoe.Component).RecoverSessions(ctx); err != nil {
							mainLog.Error("IPoE session recovery failed", "error", err)
						}

						mainLog.Info("VPP recovery: recovering PPPoE sessions")
						if err := pppoeComp.(*pppoe.Component).RecoverSessions(ctx); err != nil {
							mainLog.Error("PPPoE session recovery failed", "error", err)
						}
					}

					return nil
				},
			})
			wd.Register(vppTarget, buildRunnerConfig(wdCfg, wdCfg.Targets["vpp"]))
		}

		if tc := wdCfg.Targets["frr"]; tc == nil || tc.Enabled {
			wd.Register(targets.NewFRRTarget("", isCritical(wdCfg.Targets["frr"])), buildRunnerConfig(wdCfg, wdCfg.Targets["frr"]))
		}
	}

	showRegistry := show.NewRegistry()
	operRegistry := oper.NewRegistry()

	gatewayAddr := "0.0.0.0:50050"
	if cfg.API.Address != "" {
		gatewayAddr = cfg.API.Address
	}
	gatewayComp, err := gateway.New(coreDeps, showRegistry, operRegistry, subscriberComp.(*subscriber.Component), configd, gatewayAddr)
	if err != nil {
		log.Fatalf("Failed to create gateway component: %v", err)
	}

	collectorRegistry := state.DefaultRegistry()

	collectInterval := 5 * time.Second
	if cfg.Monitoring.CollectInterval > 0 {
		collectInterval = cfg.Monitoring.CollectInterval
	}

	monitorComp := monitor.New(monitor.Config{
		Cache:             cache,
		CollectorRegistry: collectorRegistry,
		CollectorConfig: state.CollectorConfig{
			Interval:   collectInterval,
			TTL:        30 * time.Second,
			PathPrefix: "osvbng:state:",
		},
		DisabledCollectors: cfg.Monitoring.DisabledCollectors,
		ShowRegistry:       *showRegistry,
		ConfigMgr:          configd,
	})

	orch := component.NewOrchestrator()
	orch.Register(aaaComp)
	orch.Register(routingComp)
	orch.Register(dataplaneComp)
	orch.Register(ipoeComp)
	orch.Register(subscriberComp)
	orch.Register(arpComp)
	orch.Register(pppoeComp)
	orch.Register(monitorComp)
	orch.Register(gatewayComp)
	if wd != nil {
		orch.Register(wd)
	}

	pluginComponents, err := component.LoadAll(coreDeps)
	if err != nil {
		log.Fatalf("Failed to load plugin components: %v", err)
	}

	pluginComponentsMap := make(map[string]component.Component)
	for _, comp := range pluginComponents {
		if comp != nil {
			mainLog.Info("Loaded plugin component", "name", comp.Name())
			orch.Register(comp)
			pluginComponentsMap[comp.Name()] = comp
		}
	}

	configd.AutoRegisterHandlers(&deps.ConfDeps{
		DataplaneState:   configd.GetDataplaneState(),
		Southbound:       vpp,
		AAA:              aaaComp.(*aaa.Component),
		Routing:          routingComp.(*routing.Component),
		VRFManager:       vrfMgr,
		SvcGroupResolver: svcGroupResolver,
		CPPM:             cppmManager,
		PluginComponents: pluginComponentsMap,
	})

	showRegistry.AutoRegisterAll(&deps.ShowDeps{
		Subscriber:       subscriberComp.(*subscriber.Component),
		Southbound:       coreDeps.Southbound,
		Routing:          routingComp.(*routing.Component),
		VRFManager:       vrfMgr,
		SvcGroupResolver: svcGroupResolver,
		Cache:            cache,
		OpDB:             opdbStore,
		CPPM:             cppmManager,
		Watchdog:         wd,
		PluginComponents: pluginComponentsMap,
	})

	operRegistry.AutoRegisterAll(&deps.OperDeps{
		Subscriber:       subscriberComp.(*subscriber.Component),
		PluginComponents: pluginComponentsMap,
	})

	if apiComp, ok := pluginComponentsMap["northbound.api"]; ok {
		if apiPlugin, ok := apiComp.(interface{ SetRegistries(*northbound.Adapter) }); ok {
			adapter := northbound.NewAdapter(showRegistry, configd.GetRegistry(), operRegistry, configd)
			apiPlugin.SetRegistries(adapter)
		}
		if wd != nil {
			if hp, ok := apiComp.(interface{ SetHealthEndpoints(*watchdog.Watchdog) }); ok {
				hp.SetHealthEndpoints(wd)
			}
		}
	}

	ctx := context.Background()
	if err := orch.Start(ctx); err != nil {
		log.Fatalf("Failed to start components: %v", err)
	}

	mainLog.Info("osvbng started successfully")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	mainLog.Info("Shutting down osvbng...")

	if err := orch.Stop(ctx); err != nil {
		mainLog.Error("Error stopping components", "error", err)
	}

	if err := vpp.Close(); err != nil {
		mainLog.Error("Error closing VPP connection", "error", err)
	}

	mainLog.Info("osvbng stopped")
}

func isCritical(tc *syscfg.WatchdogTargetConfig) bool {
	if tc == nil || tc.Critical == nil {
		return false
	}
	return *tc.Critical
}

func buildRunnerConfig(wdCfg syscfg.WatchdogConfig, tc *syscfg.WatchdogTargetConfig) watchdog.RunnerConfig {
	rc := watchdog.RunnerConfig{
		CheckInterval:       wdCfg.CheckInterval,
		Timeout:             3 * time.Second,
		FailureThreshold:    3,
		OnFailure:           watchdog.ActionWarn,
		ReconnectBackoff:    1 * time.Second,
		ReconnectMaxBackoff: 30 * time.Second,
		FailExitCode:        1,
	}

	if tc == nil {
		return rc
	}

	if tc.Timeout > 0 {
		rc.Timeout = tc.Timeout
	}
	if tc.FailureThreshold > 0 {
		rc.FailureThreshold = tc.FailureThreshold
	}
	if tc.OnFailure != "" {
		rc.OnFailure = watchdog.FailureAction(tc.OnFailure)
	}
	if tc.MinRestartInterval > 0 {
		rc.MinRestartInterval = tc.MinRestartInterval
	}
	if tc.ReconnectBackoff > 0 {
		rc.ReconnectBackoff = tc.ReconnectBackoff
	}
	if tc.ReconnectMaxBackoff > 0 {
		rc.ReconnectMaxBackoff = tc.ReconnectMaxBackoff
	}
	if tc.ReconnectMaxRetries > 0 {
		rc.ReconnectMaxRetries = tc.ReconnectMaxRetries
	}
	if tc.FailExitCode != 0 {
		rc.FailExitCode = tc.FailExitCode
	}
	if tc.FailDelay > 0 {
		rc.FailDelay = tc.FailDelay
	}

	return rc
}

func bootstrapDataplane(
	log *slog.Logger,
	configd *configmgr.ConfigManager,
	sb *vpp.VPP,
	vrfMgr *vrfmgr.Manager,
	svcGroupResolver *svcgroup.Resolver,
	cppmManager *cppm.Manager,
	cfg *config.Config,
	accessInterface string,
) error {
	configd.ResetForRecovery()
	vrfMgr.Reset()

	log.Info("Applying startup configuration")
	if err := configd.ApplyLoadedConfig(); err != nil {
		if err.Error() != "failed to commit: no changes to commit" {
			return fmt.Errorf("apply startup config: %w", err)
		}
	}

	if err := sb.LoadInterfaces(); err != nil {
		log.Warn("Failed to load interfaces", "error", err)
	}
	if err := sb.LoadIPState(); err != nil {
		log.Warn("Failed to load IP state", "error", err)
	}

	if err := configd.LoadFromDataplane(sb); err != nil {
		return fmt.Errorf("load dataplane state: %w", err)
	}

	configd.AutoRegisterHandlers(&deps.ConfDeps{
		DataplaneState:   configd.GetDataplaneState(),
		Southbound:       sb,
		CPPM:             cppmManager,
		VRFManager:       vrfMgr,
		SvcGroupResolver: svcGroupResolver,
	})

	if err := sb.AddLocalRoute("255.255.255.255/32"); err != nil {
		log.Warn("Failed to add broadcast route", "error", err)
	}

	log.Info("Applying infrastructure configuration")
	infraSess, err := configd.CreateCandidateSession()
	if err != nil {
		return fmt.Errorf("create infra session: %w", err)
	}
	if err := configd.ApplyInfrastructureConfig(infraSess, cfg, accessInterface); err != nil {
		configd.CloseCandidateSession(infraSess)
		return fmt.Errorf("apply infrastructure config: %w", err)
	}
	if err := configd.Commit(infraSess); err != nil {
		if err.Error() != "no changes to commit" {
			configd.CloseCandidateSession(infraSess)
			return fmt.Errorf("commit infrastructure config: %w", err)
		}
	}
	configd.CloseCandidateSession(infraSess)

	log.Info("Dataplane bootstrap complete")
	return nil
}
