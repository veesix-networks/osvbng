package http

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/veesix-networks/osvbng/pkg/auth"
)

func (p *Provider) StartAccounting(ctx context.Context, session *auth.Session) error {
	if p.cfg.Accounting == nil || !p.cfg.Accounting.Enabled {
		return nil
	}
	return p.sendAccountingEvent(ctx, "start", session)
}

func (p *Provider) UpdateAccounting(ctx context.Context, session *auth.Session) error {
	if p.cfg.Accounting == nil || !p.cfg.Accounting.Enabled {
		return nil
	}
	return p.sendAccountingEvent(ctx, "update", session)
}

func (p *Provider) StopAccounting(ctx context.Context, session *auth.Session) error {
	if p.cfg.Accounting == nil || !p.cfg.Accounting.Enabled {
		return nil
	}
	return p.sendAccountingEvent(ctx, "stop", session)
}

func (p *Provider) sendAccountingEvent(ctx context.Context, event string, session *auth.Session) error {
	endpoint, template := p.getAccountingConfig(event)
	if endpoint == "" {
		endpoint = p.cfg.Endpoint
	}

	endpointTmpl, err := parseTemplate("acct_endpoint", endpoint)
	if err != nil {
		p.logger.Error("Failed to parse accounting endpoint template",
			"event", event,
			"error", err)
		return nil
	}

	if template == "" {
		template = DefaultAccountingTemplate
	}
	bodyTmpl, err := parseTemplate("acct_body", template)
	if err != nil {
		p.logger.Error("Failed to parse accounting body template",
			"event", event,
			"error", err)
		return nil
	}

	acctCtx := &AccountingTemplateContext{
		Event:           event,
		SessionID:       session.SessionID,
		AcctSessionID:   session.AcctSessionID,
		Username:        session.Username,
		MAC:             session.MAC,
		RxBytes:         session.RxBytes,
		TxBytes:         session.TxBytes,
		RxPackets:       session.RxPackets,
		TxPackets:       session.TxPackets,
		SessionDuration: session.SessionDuration,
		Attributes:      session.Attributes,
	}

	tmplCtx := &TemplateContext{
		Username:      session.Username,
		MAC:           session.MAC,
		AcctSessionID: session.AcctSessionID,
		Attributes:    session.Attributes,
	}
	if p.globalCfg != nil {
		tmplCtx.DeviceID = p.globalCfg.AAA.NASIdentifier
		tmplCtx.DeviceIP = p.globalCfg.AAA.NASIP
		tmplCtx.NASIdentifier = p.globalCfg.AAA.NASIdentifier
		tmplCtx.NASIPAddress = p.globalCfg.AAA.NASIP
	}

	renderedEndpoint, err := endpointTmpl.Execute(tmplCtx)
	if err != nil {
		p.logger.Error("Failed to render accounting endpoint",
			"event", event,
			"error", err)
		return nil
	}

	body, err := bodyTmpl.ExecuteAccounting(acctCtx)
	if err != nil {
		p.logger.Error("Failed to render accounting body",
			"event", event,
			"error", err)
		return nil
	}

	method := p.getAccountingMethod(event)

	p.logger.Debug("Sending accounting event",
		"event", event,
		"endpoint", renderedEndpoint,
		"session_id", session.SessionID,
		"username", session.Username)

	req, err := http.NewRequestWithContext(ctx, method, renderedEndpoint, strings.NewReader(body))
	if err != nil {
		p.logger.Error("Failed to create accounting request",
			"event", event,
			"error", err)
		return nil
	}

	p.setRequestHeaders(req)

	resp, err := p.client.Do(req)
	if err != nil {
		p.logger.Error("Accounting request failed",
			"event", event,
			"error", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		p.logger.Warn("Accounting request returned error status",
			"event", event,
			"status", resp.StatusCode,
			"session_id", session.SessionID)
	} else {
		p.logger.Debug("Accounting event sent successfully",
			"event", event,
			"status", resp.StatusCode,
			"session_id", session.SessionID)
	}

	return nil
}

func (p *Provider) getAccountingConfig(event string) (endpoint, template string) {
	if p.cfg.Accounting == nil {
		return "", ""
	}

	var eventCfg *AccountingEventConfig
	switch event {
	case "start":
		eventCfg = p.cfg.Accounting.Start
	case "update":
		eventCfg = p.cfg.Accounting.Update
	case "stop":
		eventCfg = p.cfg.Accounting.Stop
	}

	if eventCfg == nil {
		return "", ""
	}

	return eventCfg.Endpoint, eventCfg.Template
}

func (p *Provider) getAccountingMethod(event string) string {
	if p.cfg.Accounting == nil {
		return p.cfg.Method
	}

	var eventCfg *AccountingEventConfig
	switch event {
	case "start":
		eventCfg = p.cfg.Accounting.Start
	case "update":
		eventCfg = p.cfg.Accounting.Update
	case "stop":
		eventCfg = p.cfg.Accounting.Stop
	}

	if eventCfg != nil && eventCfg.Method != "" {
		return eventCfg.Method
	}

	return p.cfg.Method
}

func FormatError(event string, err error) string {
	return fmt.Sprintf("accounting %s failed: %v", event, err)
}
