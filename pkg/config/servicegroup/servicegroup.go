package servicegroup

type Config struct {
	VRF        string     `json:"vrf,omitempty" yaml:"vrf,omitempty"`
	Unnumbered string     `json:"unnumbered,omitempty" yaml:"unnumbered,omitempty"`
	URPF       string     `json:"urpf,omitempty" yaml:"urpf,omitempty"`
	ACL        *ACLConfig `json:"acl,omitempty" yaml:"acl,omitempty"`
	QoS        *QoSConfig `json:"qos,omitempty" yaml:"qos,omitempty"`
	Pool       string     `json:"pool,omitempty" yaml:"pool,omitempty"`
	IANAPool   string     `json:"iana-pool,omitempty" yaml:"iana-pool,omitempty"`
	PDPool     string     `json:"pd-pool,omitempty" yaml:"pd-pool,omitempty"`
}

type ACLConfig struct {
	Ingress string `json:"ingress,omitempty" yaml:"ingress,omitempty"`
	Egress  string `json:"egress,omitempty" yaml:"egress,omitempty"`
}

type QoSConfig struct {
	IngressPolicy string `json:"ingress-policy,omitempty" yaml:"ingress-policy,omitempty"`
	EgressPolicy  string `json:"egress-policy,omitempty" yaml:"egress-policy,omitempty"`
	UploadRate    uint64 `json:"upload-rate,omitempty" yaml:"upload-rate,omitempty"`
	DownloadRate  uint64 `json:"download-rate,omitempty" yaml:"download-rate,omitempty"`
}
