package prometheus

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/configmgr"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/netbind"
	"github.com/veesix-networks/osvbng/plugins/exporter/prometheus/metrics"
	"github.com/veesix-networks/osvbng/plugins/exporter/prometheus/show"

	_ "github.com/veesix-networks/osvbng/plugins/exporter/prometheus/cli"
	_ "github.com/veesix-networks/osvbng/plugins/exporter/prometheus/conf"
	_ "github.com/veesix-networks/osvbng/plugins/exporter/prometheus/show"
)

func init() {
	component.Register("exporter.prometheus", New)
}

type Component struct {
	*component.Base
	logger        *logger.Logger
	cache         cache.Cache
	cfg           *Config
	server        *http.Server
	mu            sync.RWMutex
	handlerCount  int
	serverRunning bool
}

func (c *Component) Addr() string {
	if c.cfg.ListenAddress != "" {
		return c.cfg.ListenAddress
	}
	return ":9090"
}

func (c *Component) GetStatus() *show.Status {
	c.mu.RLock()
	defer c.mu.RUnlock()

	state := "stopped"
	if c.serverRunning {
		state = "running"
	}

	return &show.Status{
		State:         state,
		ListenAddress: c.Addr(),
		HandlerCount:  c.handlerCount,
		ServerRunning: c.serverRunning,
	}
}

func New(deps component.Dependencies) (component.Component, error) {
	pluginCfgRaw, ok := configmgr.GetPluginConfig(Namespace)
	if !ok {
		return nil, nil
	}

	pluginCfg, ok := pluginCfgRaw.(*Config)
	if !ok {
		return nil, nil
	}

	if !pluginCfg.Enabled {
		return nil, nil
	}

	if _, err := pluginCfg.TLS.BuildTLSConfig(); err != nil {
		return nil, fmt.Errorf("%s: %w", Namespace, err)
	}

	return &Component{
		Base:   component.NewBaseAsync(Namespace),
		logger: logger.Get(Namespace),
		cache:  deps.Cache,
		cfg:    pluginCfg,
	}, nil
}

func (c *Component) Start(ctx context.Context) error {
	c.StartContext(ctx)
	c.logger.Info("Starting Prometheus exporter", "addr", c.Addr())

	c.Go(func() {
		c.startServer()
	})

	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	c.logger.Info("Stopping Prometheus exporter")

	if c.server != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		c.server.Shutdown(shutdownCtx)
	}

	c.mu.Lock()
	c.serverRunning = false
	c.mu.Unlock()

	c.StopContext()
	return nil
}

type prometheusCollector struct {
	cache    cache.Cache
	logger   *logger.Logger
	handlers []metrics.MetricHandler
}

func (pc *prometheusCollector) Describe(ch chan<- *prometheus.Desc) {
	pc.logger.Debug("Describing metrics")
	for _, handler := range pc.handlers {
		pc.logger.Debug("Describing handler", "name", handler.Name())
		handler.Describe(ch)
	}
	pc.logger.Debug("Describe complete")
}

func (pc *prometheusCollector) Collect(ch chan<- prometheus.Metric) {
	pc.logger.Debug("Collecting metrics")
	ctx := context.Background()
	for _, handler := range pc.handlers {
		pc.logger.Debug("Collecting from handler", "name", handler.Name())
		if err := handler.Collect(ctx, pc.cache, ch); err != nil {
			if strings.Contains(err.Error(), "key not found") {
				pc.logger.Debug("No cached data for handler", "handler", handler.Name())
			} else {
				pc.logger.Error("Failed to collect metrics", "handler", handler.Name(), "error", err)
			}
		}
	}
	pc.logger.Debug("Collect complete")
}

func (c *Component) startServer() {
	handlers, err := metrics.DefaultRegistry().CreateHandlers(c.logger)
	if err != nil {
		c.logger.Error("Failed to create metric handlers", "error", err)
		return
	}

	c.mu.Lock()
	c.handlerCount = len(handlers)
	c.mu.Unlock()

	c.logger.Info("Registered metric handlers", "count", len(handlers))

	promCollector := &prometheusCollector{
		cache:    c.cache,
		logger:   c.logger,
		handlers: handlers,
	}

	mux := http.NewServeMux()
	registry := prometheus.NewRegistry()
	registry.MustRegister(promCollector)

	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

	addr := c.Addr()
	binding := c.cfg.ListenerBinding.Resolve()
	tlsCfg, err := c.cfg.TLS.BuildTLSConfig()
	if err != nil {
		c.logger.Error("Failed to build TLS config", "error", err)
		return
	}

	c.server = &http.Server{
		Addr:      addr,
		Handler:   mux,
		TLSConfig: tlsCfg,
	}

	ln, err := netbind.ListenTCP(c.Ctx, "tcp", addr, binding)
	if err != nil {
		c.logger.Error("Failed to bind Prometheus HTTP server", "addr", addr, "binding", binding.String(), "error", err)
		return
	}

	c.mu.Lock()
	c.serverRunning = true
	c.mu.Unlock()

	if tlsCfg != nil {
		ln = tls.NewListener(ln, tlsCfg)
		c.logger.Info("Prometheus HTTPS server listening", "addr", addr, "binding", binding.String())
	} else {
		c.logger.Warn("Prometheus HTTP server listening unencrypted; set tls.cert_file + tls.key_file in production",
			"addr", addr, "binding", binding.String())
	}
	c.SignalReady()
	if err := c.server.Serve(ln); err != nil && err != http.ErrServerClosed {
		c.logger.Error("Prometheus HTTP server error", "error", err)
		c.mu.Lock()
		c.serverRunning = false
		c.mu.Unlock()
	}
}
