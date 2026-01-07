package local

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/veesix-networks/osvbng/pkg/auth"
	"github.com/veesix-networks/osvbng/pkg/conf"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/provider"
)

var globalProvider *Provider

type Provider struct {
	db     *sql.DB
	logger *slog.Logger
}

func New() (auth.AuthProvider, error) {
	pluginCfgRaw, ok := conf.GetPluginConfig(Namespace)
	if !ok {
		return nil, nil
	}

	pluginCfg, ok := pluginCfgRaw.(*Config)
	if !ok {
		return nil, fmt.Errorf("invalid config type for %s", Namespace)
	}

	dbPath := pluginCfg.DatabasePath
	if dbPath == "" {
		dbPath = "/var/lib/osvbng/local-auth.db"
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := initSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	p := &Provider{
		db:     db,
		logger: logger.Component(Namespace),
	}

	globalProvider = p

	p.logger.Info("Local auth provider initialized", "database", dbPath)
	return p, nil
}

func (p *Provider) Info() provider.Info {
	return provider.Info{
		Name:    "local",
		Version: "1.0.0",
		Author:  "OSVBNG Core",
	}
}

func (p *Provider) Authenticate(ctx context.Context, req *auth.AuthRequest) (*auth.AuthResponse, error) {
	user, err := getUserByUsername(p.db, req.Username)
	if err != nil {
		p.logger.Debug("User not found", "username", req.Username, "error", err)
		return &auth.AuthResponse{Allowed: false}, nil
	}

	if !user.Enabled {
		p.logger.Debug("User disabled", "username", req.Username)
		return &auth.AuthResponse{Allowed: false}, nil
	}

	if user.Password != nil {
		reqPassword, ok := req.Attributes["password"]
		if !ok || reqPassword != *user.Password {
			p.logger.Debug("Password mismatch", "username", req.Username)
			return &auth.AuthResponse{Allowed: false}, nil
		}
	}

	attrs, err := loadMergedAttributes(p.db, user.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to load attributes: %w", err)
	}

	p.logger.Info("User authenticated", "username", req.Username, "attributes", len(attrs))
	return &auth.AuthResponse{
		Allowed:    true,
		Attributes: attrs,
	}, nil
}

func (p *Provider) StartAccounting(ctx context.Context, session *auth.Session) error {
	return nil
}

func (p *Provider) UpdateAccounting(ctx context.Context, session *auth.Session) error {
	return nil
}

func (p *Provider) StopAccounting(ctx context.Context, session *auth.Session) error {
	return nil
}

func (p *Provider) Close() error {
	if p.db != nil {
		return p.db.Close()
	}
	return nil
}

func GetProvider() *Provider {
	return globalProvider
}

func (p *Provider) GetDB() *sql.DB {
	return p.db
}
