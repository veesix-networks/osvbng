package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"sync"
	"time"

	"github.com/veesix-networks/osvbng/internal/watchdog"
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/configmgr"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/northbound"
)

type Component struct {
	*component.Base
	logger   *logger.Logger
	adapter  *northbound.Adapter
	addr     string
	server   *http.Server
	watchdog *watchdog.Watchdog
	mu       sync.RWMutex
	running  bool
	specJSON []byte
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
		Base:   component.NewBaseAsync(Namespace),
		logger: logger.Get(Namespace),
		addr:   addr,
	}, nil
}

func (c *Component) SetRegistries(adapter *northbound.Adapter) {
	c.adapter = adapter
}

func (c *Component) SetHealthEndpoints(wd *watchdog.Watchdog) {
	c.watchdog = wd
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
	spec := buildOpenAPISpec(c.adapter)
	specData, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		c.logger.Error("Failed to marshal OpenAPI spec", "error", err)
	} else {
		c.specJSON = specData
		c.logger.Info("OpenAPI spec generated", "paths", spec.Paths.Len())
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/running-config", c.handleRunningConfig)
	mux.HandleFunc("GET /api/startup-config", c.handleStartupConfig)
	mux.HandleFunc("GET /api/show/{path...}", c.handleShow)
	mux.HandleFunc("POST /api/set/{path...}", c.handleSet)
	mux.HandleFunc("POST /api/exec/{path...}", c.handleExec)
	mux.HandleFunc("GET /api/openapi.json", c.handleOpenAPISpec)
	mux.HandleFunc("GET /api", c.handlePaths)

	swaggerFS, err := fs.Sub(swaggerUIAssets, "swagger-ui")
	if err != nil {
		c.logger.Error("Failed to create swagger UI sub-filesystem", "error", err)
	} else {
		mux.Handle("GET /api/docs/", http.StripPrefix("/api/docs/", http.FileServer(http.FS(swaggerFS))))
	}
	mux.HandleFunc("GET /api/docs", c.handleDocsRedirect)

	if os.Getenv("OSVBNG_PROFILE") != "" {
		mux.HandleFunc("GET /debug/pprof/", pprof.Index)
		mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)
		c.logger.Info("pprof debug endpoints enabled")
	}

	if c.watchdog != nil {
		mux.HandleFunc("GET /healthz", watchdog.HealthzHandler(c.watchdog))
		mux.HandleFunc("GET /readyz", watchdog.ReadyzHandler(c.watchdog))
	}

	c.server = &http.Server{
		Addr:    c.addr,
		Handler: mux,
	}

	ln, err := net.Listen("tcp", c.addr)
	if err != nil {
		c.logger.Error("Failed to bind API server", "addr", c.addr, "error", err)
		return
	}

	c.mu.Lock()
	c.running = true
	c.mu.Unlock()

	c.logger.Info("API server listening", "addr", c.addr)
	c.SignalReady()
	if err := c.server.Serve(ln); err != nil && err != http.ErrServerClosed {
		c.logger.Error("API server error", "error", err)
		c.mu.Lock()
		c.running = false
		c.mu.Unlock()
	}
}
