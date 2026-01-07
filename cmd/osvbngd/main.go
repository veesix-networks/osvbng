package main

import (
	"context"
	"flag"
	"log"
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
	"github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/internal/subscriber"
	"github.com/veesix-networks/osvbng/pkg/auth"
	"github.com/veesix-networks/osvbng/pkg/bootstrap"
	"github.com/veesix-networks/osvbng/pkg/cache/memory"
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/conf"
	confhandlers "github.com/veesix-networks/osvbng/pkg/conf/handlers"
	_ "github.com/veesix-networks/osvbng/pkg/conf/handlers/all"
	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/dhcp4"
	"github.com/veesix-networks/osvbng/pkg/events/local"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/northbound"
	"github.com/veesix-networks/osvbng/pkg/operations"
	operhandlers "github.com/veesix-networks/osvbng/pkg/oper/handlers"
	"github.com/veesix-networks/osvbng/pkg/show/handlers"
	_ "github.com/veesix-networks/osvbng/pkg/show/handlers/all"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/state"
	_ "github.com/veesix-networks/osvbng/plugins/all"
	"go.fd.io/govpp"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "Path to configuration file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	logger.Configure(cfg.Logging.Format, cfg.Logging.Level, cfg.Logging.Components)

	mainLog := logger.Component(logger.ComponentMain)
	mainLog.Info("Starting osvbng", "bng_id", cfg.Redundancy.BNGID)

	apiSocket := cfg.Dataplane.VPPAPISocket
	if apiSocket == "" {
		apiSocket = "/run/vpp/api.sock"
	}

	vppConn, err := govpp.Connect(apiSocket)
	if err != nil {
		log.Fatalf("Failed to connect to VPP: %v", err)
	}

	vppDataplane := operations.NewVPPDataplane(vppConn)

	configd := conf.NewConfigDaemon()

	if err := configd.LoadVersions(); err != nil {
		mainLog.Warn("Failed to load config versions", "error", err)
	}

	configd.AutoRegisterHandlers(&confhandlers.ConfDeps{
		Dataplane:        vppDataplane,
		AAA:              nil,
		Routing:          nil,
		PluginComponents: nil,
	})

	mainLog.Info("Applying startup configuration")
	if err := configd.ApplyStartupConfig(*configPath); err != nil {
		log.Fatalf("Failed to apply startup config: %v", err)
	}
	mainLog.Info("Startup configuration applied")

	vpp, err := southbound.NewVPP(southbound.VPPConfig{
		Connection:      vppConn,
		ParentInterface: cfg.Dataplane.AccessInterface,
	})
	if err != nil {
		log.Fatalf("Failed to create VPP southbound: %v", err)
	}

	mainLog.Info("Waiting for VPP LCP to sync interfaces...")
	time.Sleep(5 * time.Second)

	if err := vpp.SetupMemifDataplane(0, cfg.Dataplane.AccessInterface); err != nil {
		log.Fatalf("Failed to setup memif dataplane: %v", err)
	}
	mainLog.Info("Memif dataplane configured")

	if err := vpp.SetVirtualMAC(cfg.Redundancy.VirtualMAC); err != nil {
		log.Fatalf("Failed to set virtual MAC: %v", err)
	}

	if cfg.Dataplane.CPEgressInterface != "" {
		if err := vpp.SetupCPEgressInterface(cfg.Dataplane.CPEgressInterface, cfg.Dataplane.AccessInterface); err != nil {
			log.Fatalf("Failed to setup CP egress interface: %v", err)
		}
		mainLog.Info("CP egress interface configured", "interface", cfg.Dataplane.CPEgressInterface)
	}

	// VPP blackholes 255.255.255.255/32 as per default FIB implementation, this is a temp workaround for now, there are many better solutions but require more time
	if err := vpp.AddLocalRoute("255.255.255.255/32"); err != nil {
		log.Fatalf("Failed to add broadcast route: %v", err)
	}
	mainLog.Info("Added broadcast route for DHCP")

	// We will move bootstrap under config handlers at some point, or abstract the config into a generic subscriber template language?
	bootstrapper := bootstrap.New(vpp, cfg)
	if err := bootstrapper.ProvisionInfrastructure(); err != nil {
		log.Fatalf("Failed to provision infrastructure: %v", err)
	}

	socketPath := cfg.Dataplane.PuntSocketPath
	if socketPath == "" {
		socketPath = "/run/vpp/osvbng-punt.sock"
	}

	if err := vpp.RegisterPuntSocket(socketPath, 67, cfg.Dataplane.AccessInterface); err != nil {
		mainLog.Warn("Failed to register punt socket for UDP 67", "error", err)
	}

	if err := vpp.RegisterPuntSocket(socketPath, 68, cfg.Dataplane.AccessInterface); err != nil {
		mainLog.Warn("Failed to register punt socket for UDP 68", "error", err)
	}

	if err := vpp.EnableDirectedBroadcast(cfg.Dataplane.AccessInterface); err != nil {
		mainLog.Warn("Failed to enable directed broadcast", "interface", cfg.Dataplane.AccessInterface, "error", err)
	}

	eventBus := local.NewBus()
	cache := memory.New()

	deps := component.Dependencies{
		EventBus: eventBus,
		Cache:    cache,
		VPP:      vpp,
		Config:   cfg,
	}

	authProviderName := cfg.AAA.Provider
	if authProviderName == "" {
		authProviderName = "local"
	}

	authProvider, err := auth.New(authProviderName)
	if err != nil {
		log.Fatalf("Failed to create auth provider '%s': %v", authProviderName, err)
	}

	aaaComp, err := aaa.New(deps, authProvider)
	if err != nil {
		log.Fatalf("Failed to create AAA component: %v", err)
	}

	routingComp, err := routing.New(deps)
	if err != nil {
		log.Fatalf("Failed to create routing component: %v", err)
	}

	dataplaneComp, err := dataplane.New(deps)
	if err != nil {
		log.Fatalf("Failed to create dataplane component: %v", err)
	}

	dpComp := dataplaneComp.(*dataplane.Component)
	deps.DHCPChan = dpComp.DHCPChan
	deps.ARPChan = dpComp.ARPChan
	deps.PPPChan = dpComp.PPPoEChan

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

	ipoeComp, err := ipoe.New(deps, nil, dhcp4Provider)
	if err != nil {
		log.Fatalf("Failed to create ipoe component: %v", err)
	}

	subscriberComp, err := subscriber.New(deps, nil)
	if err != nil {
		log.Fatalf("Failed to create subscriber component: %v", err)
	}

	arpComp, err := arp.New(deps, nil)
	if err != nil {
		log.Fatalf("Failed to create arp component: %v", err)
	}

	showRegistry := handlers.NewRegistry()
	operRegistry := operhandlers.NewRegistry()

	gatewayAddr := "0.0.0.0:50050"
	if cfg.API.Address != "" {
		gatewayAddr = cfg.API.Address
	}
	gatewayComp, err := gateway.New(deps, showRegistry, operRegistry, subscriberComp.(*subscriber.Component), configd, gatewayAddr)
	if err != nil {
		log.Fatalf("Failed to create gateway component: %v", err)
	}

	collectorRegistry := state.DefaultRegistry()
	deps.CollectorRegistry = collectorRegistry

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
		ShowRegistry:      showRegistry,
	})

	orch := component.NewOrchestrator()
	orch.Register(aaaComp)
	orch.Register(routingComp)
	orch.Register(dataplaneComp)
	orch.Register(ipoeComp)
	orch.Register(subscriberComp)
	orch.Register(arpComp)
	orch.Register(monitorComp)
	orch.Register(gatewayComp)

	pluginComponents, err := component.LoadAll(deps)
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

	configd.AutoRegisterHandlers(&confhandlers.ConfDeps{
		Dataplane:        vppDataplane,
		AAA:              aaaComp.(*aaa.Component),
		Routing:          routingComp.(*routing.Component),
		PluginComponents: pluginComponentsMap,
	})

	showRegistry.AutoRegisterAll(&handlers.ShowDeps{
		Subscriber:       subscriberComp.(*subscriber.Component),
		Southbound:       deps.VPP,
		Routing:          routingComp.(*routing.Component),
		Cache:            cache,
		PluginComponents: pluginComponentsMap,
	})

	operRegistry.AutoRegisterAll(&operhandlers.OperDeps{
		PluginComponents: pluginComponentsMap,
	})

	if apiComp, ok := pluginComponentsMap["northbound.api"]; ok {
		if apiPlugin, ok := apiComp.(interface{ SetRegistries(*northbound.Adapter) }); ok {
			adapter := northbound.NewAdapter(showRegistry, configd.GetRegistry(), operRegistry)
			apiPlugin.SetRegistries(adapter)
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
