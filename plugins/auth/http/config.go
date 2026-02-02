package http

import (
	"time"

	"github.com/veesix-networks/osvbng/pkg/auth"
	"github.com/veesix-networks/osvbng/pkg/configmgr"
)

const Namespace = "subscriber.auth.http"

type Config struct {
	Endpoint    string            `json:"endpoint" yaml:"endpoint"`
	Method      string            `json:"method,omitempty" yaml:"method,omitempty"`
	Timeout     time.Duration     `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	TLS         *TLSConfig        `json:"tls,omitempty" yaml:"tls,omitempty"`
	Auth        *AuthConfig       `json:"auth,omitempty" yaml:"auth,omitempty"`
	Headers     map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
	RequestBody *RequestBodyConfig `json:"request_body,omitempty" yaml:"request_body,omitempty"`
	Response    *ResponseConfig   `json:"response,omitempty" yaml:"response,omitempty"`
	Accounting  *AccountingConfig `json:"accounting,omitempty" yaml:"accounting,omitempty"`
}

type TLSConfig struct {
	InsecureSkipVerify bool   `json:"insecure_skip_verify,omitempty" yaml:"insecure_skip_verify,omitempty"`
	CACertFile         string `json:"ca_cert_file,omitempty" yaml:"ca_cert_file,omitempty"`
	CertFile           string `json:"cert_file,omitempty" yaml:"cert_file,omitempty"`
	KeyFile            string `json:"key_file,omitempty" yaml:"key_file,omitempty"`
}

type AuthConfig struct {
	Type     string `json:"type,omitempty" yaml:"type,omitempty"` // "basic" or "bearer"
	Username string `json:"username,omitempty" yaml:"username,omitempty"`
	Password string `json:"password,omitempty" yaml:"password,omitempty"`
	Token    string `json:"token,omitempty" yaml:"token,omitempty"`
}

type RequestBodyConfig struct {
	Template string `json:"template,omitempty" yaml:"template,omitempty"`
}

type ResponseConfig struct {
	AllowedCondition  *ConditionConfig       `json:"allowed_condition,omitempty" yaml:"allowed_condition,omitempty"`
	AttributeMappings []HTTPAttributeMapping `json:"attribute_mappings,omitempty" yaml:"attribute_mappings,omitempty"`
}

type ConditionConfig struct {
	JSONPath string `json:"jsonpath,omitempty" yaml:"jsonpath,omitempty"`
	Value    string `json:"value,omitempty" yaml:"value,omitempty"`
}

type HTTPAttributeMapping struct {
	Path      string `json:"path" yaml:"path"`
	Attribute string `json:"attribute" yaml:"attribute"`
}

type AccountingConfig struct {
	Enabled bool                  `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Start   *AccountingEventConfig `json:"start,omitempty" yaml:"start,omitempty"`
	Update  *AccountingEventConfig `json:"update,omitempty" yaml:"update,omitempty"`
	Stop    *AccountingEventConfig `json:"stop,omitempty" yaml:"stop,omitempty"`
}

type AccountingEventConfig struct {
	Endpoint string `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	Method   string `json:"method,omitempty" yaml:"method,omitempty"`
	Template string `json:"template,omitempty" yaml:"template,omitempty"`
}

func init() {
	configmgr.RegisterPluginConfig(Namespace, Config{})
	auth.Register("http", New)
}
