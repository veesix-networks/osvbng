package http

import (
	"bytes"
	"text/template"
)

type Template struct {
	tmpl *template.Template
}

type TemplateContext struct {
	Username      string
	MAC           string
	AcctSessionID string
	SVLAN         uint16
	CVLAN         uint16
	Interface     string
	AccessType    string
	PolicyName    string

	DeviceID  string
	DeviceIP  string

	NASIdentifier string
	NASIPAddress  string

	Attributes map[string]string
}

type AccountingTemplateContext struct {
	Event string

	SessionID       string
	AcctSessionID   string
	Username        string
	MAC             string

	RxBytes         uint64
	TxBytes         uint64
	RxPackets       uint64
	TxPackets       uint64
	SessionDuration uint32

	Attributes map[string]string
}

func parseTemplate(name, text string) (*Template, error) {
	tmpl, err := template.New(name).Parse(text)
	if err != nil {
		return nil, err
	}
	return &Template{tmpl: tmpl}, nil
}

func (t *Template) Execute(ctx *TemplateContext) (string, error) {
	var buf bytes.Buffer
	if err := t.tmpl.Execute(&buf, ctx); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (t *Template) ExecuteAccounting(ctx *AccountingTemplateContext) (string, error) {
	var buf bytes.Buffer
	if err := t.tmpl.Execute(&buf, ctx); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (t *Template) ExecuteString(ctx *TemplateContext) (string, error) {
	return t.Execute(ctx)
}
