package svcgroup

import (
	"fmt"
	"log/slog"
	"strconv"
	"sync"

	"github.com/veesix-networks/osvbng/pkg/config/servicegroup"
	"github.com/veesix-networks/osvbng/pkg/logger"
)

type ServiceGroup struct {
	Name         string
	VRF          string
	Unnumbered   string
	URPF         string
	ACLIngress   string
	ACLEgress    string
	QoSIngress   string
	QoSEgress    string
	UploadRate   uint64
	DownloadRate uint64
}

func (r ServiceGroup) LogAttrs() []slog.Attr {
	var attrs []slog.Attr
	if r.Name != "" {
		attrs = append(attrs, slog.String("service_group", r.Name))
	}
	if r.VRF != "" {
		attrs = append(attrs, slog.String("vrf", r.VRF))
	}
	if r.Unnumbered != "" {
		attrs = append(attrs, slog.String("unnumbered", r.Unnumbered))
	}
	if r.URPF != "" {
		attrs = append(attrs, slog.String("urpf", r.URPF))
	}
	if r.ACLIngress != "" {
		attrs = append(attrs, slog.String("acl_ingress", r.ACLIngress))
	}
	if r.ACLEgress != "" {
		attrs = append(attrs, slog.String("acl_egress", r.ACLEgress))
	}
	if r.QoSIngress != "" {
		attrs = append(attrs, slog.String("qos_ingress", r.QoSIngress))
	}
	if r.QoSEgress != "" {
		attrs = append(attrs, slog.String("qos_egress", r.QoSEgress))
	}
	if r.UploadRate != 0 {
		attrs = append(attrs, slog.Uint64("upload_rate", r.UploadRate))
	}
	if r.DownloadRate != 0 {
		attrs = append(attrs, slog.Uint64("download_rate", r.DownloadRate))
	}
	return attrs
}

type Resolver struct {
	mu     sync.RWMutex
	groups map[string]*servicegroup.Config
	logger *slog.Logger
}

func New() *Resolver {
	return &Resolver{
		groups: make(map[string]*servicegroup.Config),
		logger: logger.Get(logger.SvcGroup),
	}
}

func (r *Resolver) Set(name string, cfg *servicegroup.Config) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.groups[name] = cfg
	r.logger.Info("Set service group", "name", name, "vrf", cfg.VRF)
}

func (r *Resolver) Delete(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.groups, name)
	r.logger.Info("Deleted service group", "name", name)
}

func (r *Resolver) Get(name string) *servicegroup.Config {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.groups[name]
}

func (r *Resolver) GetAll() map[string]*servicegroup.Config {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]*servicegroup.Config, len(r.groups))
	for k, v := range r.groups {
		result[k] = v
	}
	return result
}

// Resolve performs three-layer merge:
// 1. Start with default service group config (if defaultSG set)
// 2. Override with AAA service group config (if sgName set)
// 3. Override with per-field AAA attributes
func (r *Resolver) Resolve(sgName, defaultSG string, aaaAttrs map[string]interface{}) ServiceGroup {
	var result ServiceGroup

	r.mu.RLock()
	defer r.mu.RUnlock()

	if defaultSG != "" {
		if cfg, ok := r.groups[defaultSG]; ok {
			result.Name = defaultSG
			applyConfig(&result, cfg)
		} else {
			r.logger.Warn("Default service group not found", "name", defaultSG)
		}
	}

	if sgName != "" {
		if cfg, ok := r.groups[sgName]; ok {
			result.Name = sgName
			applyConfig(&result, cfg)
		} else {
			r.logger.Warn("Service group not found", "name", sgName)
		}
	}

	applyAAAOverrides(&result, aaaAttrs)

	return result
}

func applyConfig(r *ServiceGroup, cfg *servicegroup.Config) {
	if cfg.VRF != "" {
		r.VRF = cfg.VRF
	}
	if cfg.Unnumbered != "" {
		r.Unnumbered = cfg.Unnumbered
	}
	if cfg.URPF != "" {
		r.URPF = cfg.URPF
	}
	if cfg.ACL != nil {
		if cfg.ACL.Ingress != "" {
			r.ACLIngress = cfg.ACL.Ingress
		}
		if cfg.ACL.Egress != "" {
			r.ACLEgress = cfg.ACL.Egress
		}
	}
	if cfg.QoS != nil {
		if cfg.QoS.IngressPolicy != "" {
			r.QoSIngress = cfg.QoS.IngressPolicy
		}
		if cfg.QoS.EgressPolicy != "" {
			r.QoSEgress = cfg.QoS.EgressPolicy
		}
		if cfg.QoS.UploadRate != 0 {
			r.UploadRate = cfg.QoS.UploadRate
		}
		if cfg.QoS.DownloadRate != 0 {
			r.DownloadRate = cfg.QoS.DownloadRate
		}
	}
}

func applyAAAOverrides(r *ServiceGroup, attrs map[string]interface{}) {
	if attrs == nil {
		return
	}
	if v := getStringAttr(attrs, "vrf"); v != "" {
		r.VRF = v
	}
	if v := getStringAttr(attrs, "unnumbered"); v != "" {
		r.Unnumbered = v
	}
	if v := getStringAttr(attrs, "urpf"); v != "" {
		r.URPF = v
	}
	if v := getStringAttr(attrs, "acl.ingress"); v != "" {
		r.ACLIngress = v
	}
	if v := getStringAttr(attrs, "acl.egress"); v != "" {
		r.ACLEgress = v
	}
	if v := getStringAttr(attrs, "qos.ingress-policy"); v != "" {
		r.QoSIngress = v
	}
	if v := getStringAttr(attrs, "qos.egress-policy"); v != "" {
		r.QoSEgress = v
	}
	if v := getUint64Attr(attrs, "qos.upload-rate"); v != 0 {
		r.UploadRate = v
	}
	if v := getUint64Attr(attrs, "qos.download-rate"); v != 0 {
		r.DownloadRate = v
	}
}

func getStringAttr(attrs map[string]interface{}, key string) string {
	v, ok := attrs[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	return s
}

func getUint64Attr(attrs map[string]interface{}, key string) uint64 {
	v, ok := attrs[key]
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case uint64:
		return val
	case float64:
		return uint64(val)
	case int:
		return uint64(val)
	case string:
		n, err := strconv.ParseUint(val, 10, 64)
		if err != nil {
			return 0
		}
		return n
	default:
		return 0
	}
}
