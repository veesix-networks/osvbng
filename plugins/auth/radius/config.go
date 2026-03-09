// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package radius

import (
	"fmt"
	"time"

	"github.com/veesix-networks/osvbng/pkg/auth"
	"github.com/veesix-networks/osvbng/pkg/configmgr"
)

const Namespace = "subscriber.auth.radius"

const (
	DefaultAuthPort      = 1812
	DefaultAcctPort      = 1813
	DefaultTimeout       = 3 * time.Second
	DefaultRetries       = 3
	DefaultDeadTime      = 30 * time.Second
	DefaultDeadThreshold = 3
	DefaultNASPortType   = "Virtual"
)

type Config struct {
	Servers          []ServerConfig   `json:"servers" yaml:"servers"`
	AuthPort         int              `json:"auth_port,omitempty" yaml:"auth_port,omitempty"`
	AcctPort         int              `json:"acct_port,omitempty" yaml:"acct_port,omitempty"`
	Timeout          time.Duration    `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	Retries          int              `json:"retries,omitempty" yaml:"retries,omitempty"`
	NASIdentifier    string           `json:"nas_identifier,omitempty" yaml:"nas_identifier,omitempty"`
	NASIP            string           `json:"nas_ip,omitempty" yaml:"nas_ip,omitempty"`
	NASPortType      string           `json:"nas_port_type,omitempty" yaml:"nas_port_type,omitempty"`
	DeadTime         time.Duration    `json:"dead_time,omitempty" yaml:"dead_time,omitempty"`
	DeadThreshold    int              `json:"dead_threshold,omitempty" yaml:"dead_threshold,omitempty"`
	Dictionaries     []string         `json:"dictionaries,omitempty" yaml:"dictionaries,omitempty"`
	ResponseMappings []CustomMapping  `json:"response_mappings,omitempty" yaml:"response_mappings,omitempty"`
	RequestMappings  []RequestMapping `json:"request_mappings,omitempty" yaml:"request_mappings,omitempty"`
}

type ServerConfig struct {
	Host   string `json:"host" yaml:"host"`
	Secret string `json:"secret" yaml:"secret"`
}

type CustomMapping struct {
	RadiusAttr string `json:"radius_attr,omitempty" yaml:"radius_attr,omitempty"`
	VendorID   uint32 `json:"vendor_id,omitempty" yaml:"vendor_id,omitempty"`
	VendorType byte   `json:"vendor_type,omitempty" yaml:"vendor_type,omitempty"`
	Internal   string `json:"internal" yaml:"internal"`
	Extract    string `json:"extract,omitempty" yaml:"extract,omitempty"`
}

type RequestMapping struct {
	Internal   string `json:"internal" yaml:"internal"`
	RadiusAttr string `json:"radius_attr" yaml:"radius_attr"`
}

func (c *Config) applyDefaults() {
	if c.AuthPort == 0 {
		c.AuthPort = DefaultAuthPort
	}
	if c.AcctPort == 0 {
		c.AcctPort = DefaultAcctPort
	}
	if c.Timeout == 0 {
		c.Timeout = DefaultTimeout
	}
	if c.Retries == 0 {
		c.Retries = DefaultRetries
	}
	if c.NASPortType == "" {
		c.NASPortType = DefaultNASPortType
	}
	if c.DeadTime == 0 {
		c.DeadTime = DefaultDeadTime
	}
	if c.DeadThreshold == 0 {
		c.DeadThreshold = DefaultDeadThreshold
	}
}

func (c *Config) validate() error {
	if len(c.Servers) == 0 {
		return fmt.Errorf("at least one RADIUS server is required")
	}
	for i, s := range c.Servers {
		if s.Host == "" {
			return fmt.Errorf("server[%d]: host is required", i)
		}
		if s.Secret == "" {
			return fmt.Errorf("server[%d]: secret is required", i)
		}
	}
	for i, m := range c.ResponseMappings {
		if m.Internal == "" {
			return fmt.Errorf("response_mappings[%d]: internal attribute is required", i)
		}
		if m.RadiusAttr == "" && m.VendorID == 0 {
			return fmt.Errorf("response_mappings[%d]: radius_attr or vendor_id is required", i)
		}
	}
	return nil
}

func init() {
	configmgr.RegisterPluginConfig(Namespace, Config{})
	auth.Register("radius", New)
}
