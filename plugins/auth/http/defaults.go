package http

const DefaultRequestBodyTemplate = `{
  "username": "{{.Username}}",
  "mac_address": "{{.MAC}}",
  "session_id": "{{.AcctSessionID}}",
  "svlan": {{.SVLAN}},
  "cvlan": {{.CVLAN}},
  "interface": "{{.Interface}}",
  "access_type": "{{.AccessType}}",
  "policy_name": "{{.PolicyName}}",
  "device_id": "{{.DeviceID}}",
  "device_ip": "{{.DeviceIP}}"
}`

const DefaultAccountingTemplate = `{
  "event": "{{.Event}}",
  "session_id": "{{.SessionID}}",
  "acct_session_id": "{{.AcctSessionID}}",
  "username": "{{.Username}}",
  "mac_address": "{{.MAC}}",
  "rx_bytes": {{.RxBytes}},
  "tx_bytes": {{.TxBytes}},
  "rx_packets": {{.RxPackets}},
  "tx_packets": {{.TxPackets}},
  "session_duration": {{.SessionDuration}}
}`

const DefaultMethod = "POST"

const DefaultTimeout = 30

var defaultAttributeMappings = map[string][]string{
	"ipv4_address": {
		"ipv4_address",
		"ip_address",
		"ip",
		"framed_ip_address",
		"subscriber.ipv4.address",
	},
	"ipv4_netmask": {
		"ipv4_netmask",
		"netmask",
		"framed_ip_netmask",
		"subscriber.ipv4.netmask",
	},
	"ipv4_gateway": {
		"ipv4_gateway",
		"gateway",
		"default_gateway",
		"subscriber.ipv4.gateway",
	},
	"dns_primary": {
		"dns_primary",
		"dns[0]",
		"dns_servers[0]",
		"subscriber.dns.primary",
	},
	"dns_secondary": {
		"dns_secondary",
		"dns[1]",
		"dns_servers[1]",
		"subscriber.dns.secondary",
	},
	"ipv6_prefix": {
		"ipv6_prefix",
		"delegated_prefix",
		"framed_ipv6_prefix",
		"subscriber.ipv6.prefix",
	},
	"ipv6_address": {
		"ipv6_address",
		"framed_ipv6_address",
		"subscriber.ipv6.address",
	},
	"ipv6_dns_primary": {
		"ipv6_dns_primary",
		"dns_v6[0]",
		"dns_servers_v6[0]",
		"subscriber.dns_v6.primary",
	},
	"ipv6_dns_secondary": {
		"ipv6_dns_secondary",
		"dns_v6[1]",
		"dns_servers_v6[1]",
		"subscriber.dns_v6.secondary",
	},
	"session_timeout": {
		"session_timeout",
		"timeout",
		"subscriber.session_timeout",
	},
	"idle_timeout": {
		"idle_timeout",
		"subscriber.idle_timeout",
	},
	"rate_limit_up": {
		"rate_limit_up",
		"upload_rate",
		"bandwidth_up",
		"subscriber.rate_limit.up",
	},
	"rate_limit_down": {
		"rate_limit_down",
		"download_rate",
		"bandwidth_down",
		"subscriber.rate_limit.down",
	},
	"vrf": {
		"vrf",
		"routing_instance",
		"subscriber.vrf",
	},
}

func getDefaultMappings() map[string][]string {
	result := make(map[string][]string)
	for k, v := range defaultAttributeMappings {
		paths := make([]string, len(v))
		copy(paths, v)
		result[k] = paths
	}
	return result
}
