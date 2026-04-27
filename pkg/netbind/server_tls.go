// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package netbind

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

type ServerTLSConfig struct {
	CertFile   string `json:"cert_file,omitempty"    yaml:"cert_file,omitempty"`
	KeyFile    string `json:"key_file,omitempty"     yaml:"key_file,omitempty"`
	CACertFile string `json:"ca_cert_file,omitempty" yaml:"ca_cert_file,omitempty"`
	ClientAuth string `json:"client_auth,omitempty"  yaml:"client_auth,omitempty"`
	MinVersion string `json:"min_version,omitempty"  yaml:"min_version,omitempty"`
}

func (c ServerTLSConfig) IsEnabled() bool {
	return c.CertFile != "" && c.KeyFile != ""
}

func (c ServerTLSConfig) Validate() error {
	if !c.IsEnabled() {
		if c.CertFile != "" || c.KeyFile != "" || c.CACertFile != "" || c.ClientAuth != "" {
			return fmt.Errorf("netbind: TLS partially configured: cert_file and key_file are both required")
		}
		return nil
	}
	if _, err := os.Stat(c.CertFile); err != nil {
		return fmt.Errorf("netbind: cert_file %q: %w", c.CertFile, err)
	}
	if _, err := os.Stat(c.KeyFile); err != nil {
		return fmt.Errorf("netbind: key_file %q: %w", c.KeyFile, err)
	}
	if c.CACertFile != "" {
		if _, err := os.Stat(c.CACertFile); err != nil {
			return fmt.Errorf("netbind: ca_cert_file %q: %w", c.CACertFile, err)
		}
	}
	if _, err := parseClientAuth(c.ClientAuth); err != nil {
		return err
	}
	if _, err := parseMinVersion(c.MinVersion); err != nil {
		return err
	}
	if c.ClientAuth == "require" && c.CACertFile == "" {
		return fmt.Errorf("netbind: client_auth=require requires ca_cert_file")
	}
	return nil
}

func (c ServerTLSConfig) BuildTLSConfig() (*tls.Config, error) {
	if !c.IsEnabled() {
		return nil, nil
	}

	cert, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("netbind: load keypair: %w", err)
	}

	min, err := parseMinVersion(c.MinVersion)
	if err != nil {
		return nil, err
	}

	out := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   min,
	}

	clientAuth, err := parseClientAuth(c.ClientAuth)
	if err != nil {
		return nil, err
	}
	out.ClientAuth = clientAuth

	if c.CACertFile != "" {
		ca, err := os.ReadFile(c.CACertFile)
		if err != nil {
			return nil, fmt.Errorf("netbind: read ca_cert_file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(ca) {
			return nil, fmt.Errorf("netbind: ca_cert_file %q: no PEM certs found", c.CACertFile)
		}
		out.ClientCAs = pool
	}

	return out, nil
}

func parseClientAuth(s string) (tls.ClientAuthType, error) {
	switch s {
	case "", "none":
		return tls.NoClientCert, nil
	case "request":
		return tls.RequestClientCert, nil
	case "verify":
		return tls.VerifyClientCertIfGiven, nil
	case "require":
		return tls.RequireAndVerifyClientCert, nil
	default:
		return 0, fmt.Errorf("netbind: client_auth %q: must be none|request|verify|require", s)
	}
}

func parseMinVersion(s string) (uint16, error) {
	switch s {
	case "", "1.3":
		return tls.VersionTLS13, nil
	case "1.2":
		return tls.VersionTLS12, nil
	default:
		return 0, fmt.Errorf("netbind: min_version %q: must be 1.2 or 1.3", s)
	}
}
