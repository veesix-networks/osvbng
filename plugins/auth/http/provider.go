package http

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/veesix-networks/osvbng/pkg/auth"
	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/configmgr"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/provider"
)

type Provider struct {
	cfg             *Config
	globalCfg       *config.Config
	client          *http.Client
	logger          *slog.Logger
	endpointTmpl    *Template
	requestBodyTmpl *Template
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

	if pluginCfg.Endpoint == "" {
		return nil, fmt.Errorf("endpoint is required for HTTP auth provider")
	}

	if pluginCfg.Method == "" {
		pluginCfg.Method = DefaultMethod
	}
	if pluginCfg.Timeout == 0 {
		pluginCfg.Timeout = time.Duration(DefaultTimeout) * time.Second
	}

	p := &Provider{
		cfg:       pluginCfg,
		globalCfg: cfg,
		logger:    logger.Component(Namespace),
	}

	client, err := p.buildHTTPClient()
	if err != nil {
		return nil, fmt.Errorf("failed to build HTTP client: %w", err)
	}
	p.client = client

	endpointTmpl, err := parseTemplate("endpoint", pluginCfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to parse endpoint template: %w", err)
	}
	p.endpointTmpl = endpointTmpl

	bodyTemplate := DefaultRequestBodyTemplate
	if pluginCfg.RequestBody != nil && pluginCfg.RequestBody.Template != "" {
		bodyTemplate = pluginCfg.RequestBody.Template
	}
	requestBodyTmpl, err := parseTemplate("request_body", bodyTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse request body template: %w", err)
	}
	p.requestBodyTmpl = requestBodyTmpl

	p.logger.Info("HTTP auth provider initialized", "endpoint", pluginCfg.Endpoint)
	return p, nil
}

func (p *Provider) Info() provider.Info {
	return provider.Info{
		Name:    "http",
		Version: "0.0.1",
		Author:  "osvbng Core Team",
	}
}

func (p *Provider) Authenticate(ctx context.Context, req *auth.AuthRequest) (*auth.AuthResponse, error) {
	tmplCtx := p.buildTemplateContext(req)

	endpoint, err := p.endpointTmpl.Execute(tmplCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to render endpoint template: %w", err)
	}

	body, err := p.requestBodyTmpl.Execute(tmplCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to render request body template: %w", err)
	}

	p.logger.Debug("Sending auth request",
		"endpoint", endpoint,
		"method", p.cfg.Method,
		"username", req.Username,
		"mac", req.MAC)

	httpReq, err := http.NewRequestWithContext(ctx, p.cfg.Method, endpoint, strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	p.setRequestHeaders(httpReq)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	p.logger.Debug("Received auth response",
		"status", resp.StatusCode,
		"body_length", len(respBody),
		"username", req.Username)

	allowed, err := p.checkAllowedCondition(resp, respBody)
	if err != nil {
		return nil, fmt.Errorf("failed to check allowed condition: %w", err)
	}

	if !allowed {
		p.logger.Info("Authentication denied",
			"username", req.Username,
			"mac", req.MAC,
			"status", resp.StatusCode)
		return &auth.AuthResponse{Allowed: false}, nil
	}

	attrs, err := p.extractAttributes(respBody)
	if err != nil {
		p.logger.Warn("Failed to extract some attributes",
			"error", err,
			"username", req.Username)
	}

	p.logger.Info("Authentication allowed",
		"username", req.Username,
		"mac", req.MAC,
		"attributes", len(attrs))

	return &auth.AuthResponse{
		Allowed:    true,
		Attributes: attrs,
	}, nil
}

func (p *Provider) Close() error {
	return nil
}

func (p *Provider) buildTemplateContext(req *auth.AuthRequest) *TemplateContext {
	ctx := &TemplateContext{
		Username:      req.Username,
		MAC:           req.MAC,
		AcctSessionID: req.AcctSessionID,
		SVLAN:         req.SVLAN,
		CVLAN:         req.CVLAN,
		Interface:     req.Interface,
		AccessType:    req.AccessType,
		PolicyName:    req.PolicyName,
		Attributes:    req.Attributes,
	}

	if p.globalCfg != nil {
		ctx.DeviceID = p.globalCfg.AAA.NASIdentifier
		ctx.DeviceIP = p.globalCfg.AAA.NASIP
		ctx.NASIdentifier = p.globalCfg.AAA.NASIdentifier
		ctx.NASIPAddress = p.globalCfg.AAA.NASIP
	}

	if ctx.Attributes == nil {
		ctx.Attributes = make(map[string]string)
	}

	return ctx
}

func (p *Provider) buildHTTPClient() (*http.Client, error) {
	transport := &http.Transport{}

	if p.cfg.TLS != nil {
		tlsConfig, err := p.buildTLSConfig()
		if err != nil {
			return nil, err
		}
		transport.TLSClientConfig = tlsConfig
	}

	return &http.Client{
		Transport: transport,
		Timeout:   p.cfg.Timeout,
	}, nil
}

func (p *Provider) buildTLSConfig() (*tls.Config, error) {
	tlsCfg := p.cfg.TLS
	config := &tls.Config{
		InsecureSkipVerify: tlsCfg.InsecureSkipVerify,
	}

	if tlsCfg.CACertFile != "" {
		caCert, err := os.ReadFile(tlsCfg.CACertFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA cert file: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA cert")
		}
		config.RootCAs = caCertPool
	}

	if tlsCfg.CertFile != "" && tlsCfg.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(tlsCfg.CertFile, tlsCfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		config.Certificates = []tls.Certificate{cert}
	}

	return config, nil
}

func (p *Provider) setRequestHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")

	if p.cfg.Auth != nil {
		switch strings.ToLower(p.cfg.Auth.Type) {
		case "basic":
			req.SetBasicAuth(p.cfg.Auth.Username, p.cfg.Auth.Password)
		case "bearer":
			req.Header.Set("Authorization", "Bearer "+p.cfg.Auth.Token)
		}
	}

	for k, v := range p.cfg.Headers {
		req.Header.Set(k, v)
	}
}

func (p *Provider) checkAllowedCondition(resp *http.Response, body []byte) (bool, error) {
	if p.cfg.Response != nil && p.cfg.Response.AllowedCondition != nil {
		cond := p.cfg.Response.AllowedCondition
		if cond.JSONPath != "" {
			var data map[string]interface{}
			if err := json.Unmarshal(body, &data); err != nil {
				return false, fmt.Errorf("failed to parse response JSON: %w", err)
			}

			val, ok := ExtractString(data, cond.JSONPath)
			if !ok {
				return false, nil
			}

			return val == cond.Value, nil
		}
	}

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return false, nil
	default:
		return false, fmt.Errorf("unexpected HTTP status: %d", resp.StatusCode)
	}
}

func (p *Provider) extractAttributes(body []byte) (map[string]string, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("failed to parse response JSON: %w", err)
	}

	attrs := make(map[string]string)

	defaultMappings := getDefaultMappings()
	for attr, paths := range defaultMappings {
		for _, path := range paths {
			if val, ok := ExtractString(data, path); ok {
				attrs[attr] = val
				break
			}
		}
	}

	if p.cfg.Response != nil {
		for _, mapping := range p.cfg.Response.AttributeMappings {
			if val, ok := ExtractString(data, mapping.Path); ok {
				attrs[mapping.Attribute] = val
			}
		}
	}

	return attrs, nil
}
