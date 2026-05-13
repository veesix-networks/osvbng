package aaa

const (
	AttrIPv4Address         = "ipv4_address"
	AttrIPv4Netmask         = "ipv4_netmask"
	AttrIPv4Gateway         = "ipv4_gateway"
	AttrDNSPrimary          = "dns_primary"
	AttrDNSSecondary        = "dns_secondary"
	AttrIPv6Address         = "ipv6_address"
	AttrIPv6Prefix          = "ipv6_prefix"
	AttrIPv6WANPrefix       = "ipv6_wan_prefix"
	AttrIPv6DNSPrimary      = "ipv6_dns_primary"
	AttrIPv6DNSSecondary    = "ipv6_dns_secondary"
	AttrServiceGroup        = "service-group"
	AttrVRF                 = "vrf"
	AttrUnnumbered          = "unnumbered"
	AttrURPF                = "urpf"
	AttrRoutedPrefix        = "routed_prefix"
	AttrSessionTimeout      = "session_timeout"
	AttrIdleTimeout         = "idle_timeout"
	AttrAcctInterimInterval = "acct_interim_interval"
	AttrRateLimitUp         = "rate_limit_up"
	AttrRateLimitDown       = "rate_limit_down"
	AttrACLIngress          = "acl.ingress"
	AttrACLEgress           = "acl.egress"
	AttrQoSIngressPolicy    = "qos.ingress-policy"
	AttrQoSEgressPolicy     = "qos.egress-policy"
	AttrQoSUploadRate       = "qos.upload-rate"
	AttrQoSDownloadRate     = "qos.download-rate"
	AttrPool                = "pool"
	AttrIANAPool            = "iana_pool"
	AttrPDPool              = "pd_pool"
	AttrUsername            = "username"
	AttrIPv4Profile         = "ipv4-profile"
	AttrIPv6Profile         = "ipv6-profile"
)

const (
	AttrPassword      = "password"
	AttrCHAPID        = "chap-id"
	AttrCHAPChallenge = "chap-challenge"
	AttrCHAPResponse  = "chap-response"
)

const (
	AttrCircuitID = "circuit_id"
	AttrRemoteID  = "remote_id"
	AttrHostname  = "hostname"
)

// L2TP tunnel attributes (RFC 2868 and RFC 2867 — see
// components/l2tp/60-l2tpv2/IMPLEMENTATION_SPEC.md §"Attribute Mappings").
// Names are osvbng-internal semantic names; RADIUS providers map the
// RFC 2868 attribute numbers to these on receive.
const (
	AttrTunnelType            = "tunnel.type"
	AttrTunnelMediumType      = "tunnel.medium-type"
	AttrTunnelClientEndpoint  = "tunnel.client-endpoint"
	AttrTunnelServerEndpoint  = "tunnel.server-endpoint"
	AttrTunnelPassword        = "tunnel.password"
	AttrTunnelAssignmentID    = "tunnel.assignment-id"
	AttrTunnelPreference      = "tunnel.preference"
	AttrTunnelClientAuthID    = "tunnel.client-auth-id"
	AttrTunnelServerAuthID    = "tunnel.server-auth-id"
	AttrTunnelAcctConnection  = "tunnel.acct-connection"
	AttrTunnelAcctPacketsLost = "tunnel.acct-packets-lost"
)
