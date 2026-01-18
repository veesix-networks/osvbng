package prometheus

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/configmgr"
	"github.com/veesix-networks/osvbng/pkg/logger"
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
	logger        *slog.Logger
	cache         cache.Cache
	addr          string
	server        *http.Server
	mu            sync.RWMutex
	handlerCount  int
	serverRunning bool
}

func (c *Component) Addr() string {
	return c.addr
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
		ListenAddress: c.addr,
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

	addr := ":9090"
	if pluginCfg.ListenAddress != "" {
		addr = pluginCfg.ListenAddress
	}

	return &Component{
		Base:   component.NewBase(Namespace),
		logger: logger.Component(Namespace),
		cache:  deps.Cache,
		addr:   addr,
	}, nil
}

func (c *Component) Start(ctx context.Context) error {
	c.StartContext(ctx)
	c.logger.Info("Starting Prometheus exporter", "addr", c.addr)

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
	logger   *slog.Logger
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
			pc.logger.Error("Failed to collect metrics", "handler", handler.Name(), "error", err)
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

	c.server = &http.Server{
		Addr:    c.addr,
		Handler: mux,
	}

	c.mu.Lock()
	c.serverRunning = true
	c.mu.Unlock()

	c.logger.Info("Prometheus HTTP server listening", "addr", c.addr)
	if err := c.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		c.logger.Error("Prometheus HTTP server error", "error", err)
		c.mu.Lock()
		c.serverRunning = false
		c.mu.Unlock()
	}
}
