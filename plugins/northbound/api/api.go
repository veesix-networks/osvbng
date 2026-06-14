package api

import (
	"context"
	"crypto/tls"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/pprof"
	"os"
	"sync"
	"time"

	"github.com/veesix-networks/osvbng/internal/watchdog"
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/configmgr"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/netbind"
	"github.com/veesix-networks/osvbng/pkg/northbound"
)

type Component struct {
	*component.Base
	logger         *logger.Logger
	adapter        *northbound.Adapter
	cfg            *Config
	server         *http.Server
	watchdog       *watchdog.Watchdog
	mu             sync.RWMutex
	running        bool
	listenerStatus map[string]ListenerStatus
	specJSON       []byte
	specETag       string
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

	applyUDSDefaults(pluginCfg)

	return &Component{
		Base:   component.NewBaseAsync(Namespace),
		logger: logger.Get(Namespace),
		cfg:    pluginCfg,
	}, nil
}

func (c *Component) SetRegistries(adapter *northbound.Adapter) {
	c.adapter = adapter
}

func (c *Component) SetHealthEndpoints(wd *watchdog.Watchdog) {
	c.watchdog = wd
}

func (c *Component) Start(ctx context.Context) error {
	if c.adapter == nil {
		return fmt.Errorf("northbound adapter not configured")
	}

	specData, err := northbound.GenerateOpenAPISpec(c.adapter)
	if err != nil {
		return fmt.Errorf("generate OpenAPI spec: %w", err)
	}

	c.specJSON = specData.JSON
	c.specETag = specData.ETag
	c.logger.Info("OpenAPI spec generated", "paths", specData.Spec.Paths.Len(), "etag", c.specETag)

	c.StartContext(ctx)

	if err := c.cfg.validateListeners(); err != nil {
		return fmt.Errorf("northbound.api: %w", err)
	}

	listeners, err := c.cfg.resolveListeners(c.logger)
	if err != nil {
		return fmt.Errorf("northbound.api: %w", err)
	}

	c.server = &http.Server{Handler: c.newMux()}

	for i, lc := range listeners {
		i, lc := i, lc
		c.Go(func() { c.startTCPListener(i, lc) })
	}

	if c.cfg.UDS.Enabled {
		c.Go(func() {
			c.startUDSServer()
		})
	}

	if len(listeners) == 0 && !c.cfg.UDS.Enabled {
		c.logger.Warn("northbound.api: no listeners configured; API is unreachable")
	}

	c.SignalReady()
	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	c.logger.Info("Stopping API server")

	if c.server != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		c.server.Shutdown(shutdownCtx)
	}

	if c.cfg.UDS.Enabled {
		if err := os.Remove(c.cfg.UDS.Path); err != nil && !os.IsNotExist(err) {
			c.logger.Warn("Failed to remove UDS socket file", "path", c.cfg.UDS.Path, "error", err)
		}
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

	listeners := make([]ListenerStatus, 0, len(c.listenerStatus))
	for _, ls := range c.listenerStatus {
		listeners = append(listeners, ls)
	}

	return &Status{
		State:     state,
		Listeners: listeners,
		Running:   c.running,
	}
}

func (c *Component) startTCPListener(index int, lc ListenerConfig) {
	binding := lc.ListenerBinding.Resolve()

	tlsCfg, err := lc.TLS.BuildTLSConfig()
	if err != nil {
		c.logger.Error("listener TLS config invalid",
			"index", index, "address", lc.Address, "error", err)
		return
	}

	ln, err := netbind.ListenTCP(c.Ctx, "tcp", lc.Address, binding)
	if err != nil {
		c.logger.Error("listener bind failed; other listeners unaffected",
			"index", index, "address", lc.Address, "binding", binding.String(), "error", err)
		return
	}

	if tlsCfg != nil {
		ln = tls.NewListener(ln, tlsCfg)
		c.logger.Info("API listener up (HTTPS)",
			"index", index, "address", lc.Address, "binding", binding.String())
	} else {
		c.logger.Warn("API listener up unencrypted; set tls.cert_file + tls.key_file in production",
			"index", index, "address", lc.Address, "binding", binding.String())
	}

	c.recordListenerStatus(lc.Address, ListenerStatus{
		Address: lc.Address,
		VRF:     binding.VRF,
		TLS:     tlsCfg != nil,
		Running: true,
	})
	c.markRunning(true)

	if err := c.server.Serve(ln); err != nil && err != http.ErrServerClosed {
		c.logger.Error("API listener error",
			"index", index, "address", lc.Address, "error", err)
	}

	c.recordListenerStatus(lc.Address, ListenerStatus{
		Address: lc.Address,
		VRF:     binding.VRF,
		TLS:     tlsCfg != nil,
		Running: false,
	})
}

func (c *Component) recordListenerStatus(key string, status ListenerStatus) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.listenerStatus == nil {
		c.listenerStatus = make(map[string]ListenerStatus)
	}
	c.listenerStatus[key] = status
}

func (c *Component) markRunning(v bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.running = v
}

func (c *Component) startUDSServer() {
	ln, err := listenUDS(c.cfg.UDS, c.logger)
	if err != nil {
		c.logger.Error("Failed to bind UDS API listener; TCP listener unaffected",
			"path", c.cfg.UDS.Path, "error", err)
		return
	}
	c.logger.Info("API server listening on UDS", "path", c.cfg.UDS.Path)
	if err := c.server.Serve(ln); err != nil && err != http.ErrServerClosed {
		c.logger.Error("UDS API listener error", "path", c.cfg.UDS.Path, "error", err)
	}
}

func (c *Component) newMux() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/running-config", c.handleRunningConfig)
	mux.HandleFunc("GET /api/startup-config", c.handleStartupConfig)
	mux.HandleFunc("GET /api/show/running-config", c.handleShowRunningConfig)
	mux.HandleFunc("GET /api/show/startup-config", c.handleShowStartupConfig)
	mux.HandleFunc("GET /api/show/config/history", c.handleShowConfigHistory)
	mux.HandleFunc("GET /api/show/config/version/{version}", c.handleShowConfigVersion)
	mux.HandleFunc("POST /api/config/session", c.handleConfigSessionCreate)
	mux.HandleFunc("POST /api/config/session/{session_id}/set/{path...}", c.handleConfigSessionSet)
	mux.HandleFunc("POST /api/config/session/{session_id}/commit", c.handleConfigSessionCommit)
	mux.HandleFunc("POST /api/config/session/{session_id}/discard", c.handleConfigSessionDiscard)
	mux.HandleFunc("GET /api/config/session/{session_id}/diff", c.handleConfigSessionDiff)
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

	return mux
}
