package local

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"

	"github.com/veesix-networks/osvbng/pkg/auth"
	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/configmgr"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/provider"
)

var globalProvider *Provider

type Provider struct {
	db       *sql.DB
	logger   *slog.Logger
	allowAll bool
}

func New(cfg *config.Config) (auth.AuthProvider, error) {
	pluginCfgRaw, ok := configmgr.GetPluginConfig(Namespace)
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
		db:       db,
		logger:   logger.Component(Namespace),
		allowAll: pluginCfg.AllowAll,
	}

	globalProvider = p

	if p.allowAll {
		p.logger.Warn("Local auth provider initialized with allow_all=true - ALL users will be authenticated")
	} else {
		p.logger.Info("Local auth provider initialized", "database", dbPath)
	}
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
	if p.allowAll {
		p.logger.Debug("Allowing all users (allow_all=true)", "username", req.Username)
		return &auth.AuthResponse{
			Allowed:    true,
			Attributes: make(map[string]string),
		}, nil
	}

	user, err := getUserByUsername(p.db, req.Username)
	if err != nil {
		p.logger.Debug("User not found", "username", req.Username, "error", err)
		return &auth.AuthResponse{Allowed: false}, nil
	}

	if !user.Enabled {
		p.logger.Debug("User disabled", "username", req.Username)
		return &auth.AuthResponse{Allowed: false}, nil
	}

	if chapResponse, ok := req.Attributes["chap-response"]; ok {
		if user.Password == nil {
			p.logger.Debug("CHAP auth but user has no password", "username", req.Username)
			return &auth.AuthResponse{Allowed: false}, nil
		}

		chapID := req.Attributes["chap-id"]
		chapChallenge := req.Attributes["chap-challenge"]

		if !p.validateCHAP(chapID, chapChallenge, chapResponse, *user.Password) {
			p.logger.Debug("CHAP validation failed", "username", req.Username)
			return &auth.AuthResponse{Allowed: false}, nil
		}
	} else if user.Password != nil {
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

func (p *Provider) validateCHAP(idHex, challengeHex, responseHex, secret string) bool {
	id, err := hex.DecodeString(idHex)
	if err != nil || len(id) != 1 {
		return false
	}

	challenge, err := hex.DecodeString(challengeHex)
	if err != nil {
		return false
	}

	response, err := hex.DecodeString(responseHex)
	if err != nil {
		return false
	}

	h := md5.New()
	h.Write(id)
	h.Write([]byte(secret))
	h.Write(challenge)
	expected := h.Sum(nil)

	if len(response) != len(expected) {
		return false
	}
	for i := range response {
		if response[i] != expected[i] {
			return false
		}
	}
	return true
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
