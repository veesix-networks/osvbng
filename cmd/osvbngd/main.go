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
	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/configmgr"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/dhcp4"
	"github.com/veesix-networks/osvbng/pkg/events/local"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/conf/all"
	"github.com/veesix-networks/osvbng/pkg/handlers/oper"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/show/all"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/northbound"
	"github.com/veesix-networks/osvbng/pkg/operations"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/state"
	_ "github.com/veesix-networks/osvbng/plugins/all"
	"go.fd.io/govpp"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "Path to configuration file")
	flag.Parse()

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

	mainLog := logger.Component(logger.ComponentMain)

	mainLog.Info("Starting osvbng")

	apiSocket := cfg.Dataplane.DPAPISocket
	if apiSocket == "" {
		apiSocket = "/run/osvbng/dataplane_api.sock"
	}

	vppConn, err := govpp.Connect(apiSocket)
	if err != nil {
		log.Fatalf("Failed to connect to VPP: %v", err)
	}

	vppDataplane := operations.NewVPPDataplane(vppConn)

	if err := configd.LoadVersions(); err != nil {
		mainLog.Warn("Failed to load config versions", "error", err)
	}

	configd.AutoRegisterHandlers(&deps.ConfDeps{
		Dataplane:        vppDataplane,
		AAA:              nil,
		Routing:          nil,
		PluginComponents: nil,
	})

	mainLog.Info("Applying startup configuration")
	if err := configd.ApplyLoadedConfig(); err != nil {
		if err.Error() == "failed to commit: no changes to commit" {
			mainLog.Info("No startup configuration changes to apply")
		} else {
			log.Fatalf("Failed to apply startup config: %v", err)
		}
	} else {
		mainLog.Info("Startup configuration applied")
	}

	accessInterface, err := cfg.GetAccessInterface()
	if err != nil {
		log.Fatalf("Invalid access interface configuration: %v", err)
	}

	vpp, err := southbound.NewVPP(southbound.VPPConfig{
		Connection:      vppConn,
		ParentInterface: accessInterface,
		UseDPDK:         cfg.Dataplane.DPDK != nil && len(cfg.Dataplane.DPDK.Devices) > 0,
	})
	if err != nil {
		log.Fatalf("Failed to create VPP southbound: %v", err)
	}

	mainLog.Info("Waiting for VPP LCP to sync interfaces...")
	time.Sleep(5 * time.Second)

	if err := vpp.SetupMemifDataplane(0, accessInterface, cfg.Dataplane.MemifSocketPath); err != nil {
		log.Fatalf("Failed to setup memif dataplane: %v", err)
	}
	mainLog.Info("Memif dataplane configured")

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
		socketPath = "/run/osvbng/osvbng-punt.sock"
	}

	if err := vpp.RegisterPuntSocket(socketPath, 67, accessInterface); err != nil {
		mainLog.Warn("Failed to register punt socket for UDP 67 (DHCP server port)", "error", err)
	}

	if err := vpp.EnableDirectedBroadcast(accessInterface); err != nil {
		mainLog.Warn("Failed to enable directed broadcast", "interface", accessInterface, "error", err)
	}

	eventBus := local.NewBus()
	cache := memory.New()

	coreDeps := component.Dependencies{
		EventBus:      eventBus,
		Cache:         cache,
		VPP:           vpp,
		ConfigManager: configd,
	}

	authProviderName := cfg.AAA.Provider
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

	dataplaneComp, err := dataplane.New(coreDeps)
	if err != nil {
		log.Fatalf("Failed to create dataplane component: %v", err)
	}

	dpComp := dataplaneComp.(*dataplane.Component)
	coreDeps.DHCPChan = dpComp.DHCPChan
	coreDeps.ARPChan = dpComp.ARPChan
	coreDeps.PPPChan = dpComp.PPPoEChan

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

	ipoeComp, err := ipoe.New(coreDeps, nil, dhcp4Provider)
	if err != nil {
		log.Fatalf("Failed to create ipoe component: %v", err)
	}

	subscriberComp, err := subscriber.New(coreDeps, nil)
	if err != nil {
		log.Fatalf("Failed to create subscriber component: %v", err)
	}

	arpComp, err := arp.New(coreDeps, nil)
	if err != nil {
		log.Fatalf("Failed to create arp component: %v", err)
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
		ShowRegistry:      *showRegistry,
		ConfigMgr:         configd,
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
		Dataplane:        vppDataplane,
		AAA:              aaaComp.(*aaa.Component),
		Routing:          routingComp.(*routing.Component),
		PluginComponents: pluginComponentsMap,
	})

	showRegistry.AutoRegisterAll(&deps.ShowDeps{
		Subscriber:       subscriberComp.(*subscriber.Component),
		Southbound:       coreDeps.VPP,
		Routing:          routingComp.(*routing.Component),
		Cache:            cache,
		PluginComponents: pluginComponentsMap,
	})

	operRegistry.AutoRegisterAll(&deps.OperDeps{
		PluginComponents: pluginComponentsMap,
	})

	if apiComp, ok := pluginComponentsMap["northbound.api"]; ok {
		if apiPlugin, ok := apiComp.(interface{ SetRegistries(*northbound.Adapter) }); ok {
			adapter := northbound.NewAdapter(showRegistry, configd.GetRegistry(), operRegistry, configd)
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
