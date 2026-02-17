package http

import (
	"github.com/veesix-networks/osvbng/pkg/aaa"
)

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
	aaa.AttrIPv4Address: {
		"ipv4_address",
		"ip_address",
		"ip",
		"framed_ip_address",
		"subscriber.ipv4.address",
	},
	aaa.AttrIPv4Netmask: {
		"ipv4_netmask",
		"netmask",
		"framed_ip_netmask",
		"subscriber.ipv4.netmask",
	},
	aaa.AttrIPv4Gateway: {
		"ipv4_gateway",
		"gateway",
		"default_gateway",
		"subscriber.ipv4.gateway",
	},
	aaa.AttrDNSPrimary: {
		"dns_primary",
		"dns[0]",
		"dns_servers[0]",
		"subscriber.dns.primary",
	},
	aaa.AttrDNSSecondary: {
		"dns_secondary",
		"dns[1]",
		"dns_servers[1]",
		"subscriber.dns.secondary",
	},
	aaa.AttrIPv6Prefix: {
		"ipv6_prefix",
		"delegated_prefix",
		"framed_ipv6_prefix",
		"subscriber.ipv6.prefix",
	},
	aaa.AttrIPv6Address: {
		"ipv6_address",
		"framed_ipv6_address",
		"subscriber.ipv6.address",
	},
	aaa.AttrIPv6DNSPrimary: {
		"ipv6_dns_primary",
		"dns_v6[0]",
		"dns_servers_v6[0]",
		"subscriber.dns_v6.primary",
	},
	aaa.AttrIPv6DNSSecondary: {
		"ipv6_dns_secondary",
		"dns_v6[1]",
		"dns_servers_v6[1]",
		"subscriber.dns_v6.secondary",
	},
	aaa.AttrSessionTimeout: {
		"session_timeout",
		"timeout",
		"subscriber.session_timeout",
	},
	aaa.AttrIdleTimeout: {
		"idle_timeout",
		"subscriber.idle_timeout",
	},
	aaa.AttrRateLimitUp: {
		"rate_limit_up",
		"upload_rate",
		"bandwidth_up",
		"subscriber.rate_limit.up",
	},
	aaa.AttrRateLimitDown: {
		"rate_limit_down",
		"download_rate",
		"bandwidth_down",
		"subscriber.rate_limit.down",
	},
	aaa.AttrVRF: {
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
