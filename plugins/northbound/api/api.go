package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/configmgr"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/northbound"
)

type Component struct {
	*component.Base
	logger   *slog.Logger
	adapter  *northbound.Adapter
	addr     string
	server   *http.Server
	mu       sync.RWMutex
	running  bool
}

func NewComponent(deps component.Dependencies) (component.Component, error) {
	pluginCfgRaw, ok := configmgr.GetPluginConfig(Namespace)
	if !ok {
		return nil, nil
	}

	pluginCfg, ok := pluginCfgRaw.(*Config)
	if !ok {
		return nil, fmt.Errorf("invalid config type for %s", Namespace)
	}

	if !pluginCfg.Enabled {
		return nil, nil
	}

	addr := ":8080"
	if pluginCfg.ListenAddress != "" {
		addr = pluginCfg.ListenAddress
	}

	return &Component{
		Base:   component.NewBase(Namespace),
		logger: logger.Component(Namespace),
		addr:   addr,
	}, nil
}

func (c *Component) SetRegistries(adapter *northbound.Adapter) {
	c.adapter = adapter
}

func (c *Component) Start(ctx context.Context) error {
	c.StartContext(ctx)
	c.logger.Info("Starting API server", "addr", c.addr)

	c.Go(func() {
		c.startServer()
	})

	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	c.logger.Info("Stopping API server")

	if c.server != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		c.server.Shutdown(shutdownCtx)
	}

	c.mu.Lock()
	c.running = false
	c.mu.Unlock()

	c.StopContext()
	return nil
}

func (c *Component) GetStatus() *Status {
	c.mu.RLock()
	defer c.mu.RUnlock()

	state := "stopped"
	if c.running {
		state = "running"
	}

	return &Status{
		State:         state,
		ListenAddress: c.addr,
		Running:       c.running,
	}
}

func (c *Component) startServer() {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/{path...}", c.handleShow)
	mux.HandleFunc("POST /api/{path...}", c.handleConfig)
	mux.HandleFunc("GET /api", c.handlePaths)

	c.server = &http.Server{
		Addr:    c.addr,
		Handler: mux,
	}

	c.mu.Lock()
	c.running = true
	c.mu.Unlock()

	c.logger.Info("API server listening", "addr", c.addr)
	if err := c.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		c.logger.Error("API server error", "error", err)
		c.mu.Lock()
		c.running = false
		c.mu.Unlock()
	}
}
